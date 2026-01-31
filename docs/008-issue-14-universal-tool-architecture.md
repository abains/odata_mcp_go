# Issue #14: Universal Tool Architecture

**Date:** 2026-01-31
**Document ID:** 008
**Subject:** Solving tool explosion with single universal tool pattern
**Related:** GitHub Issue #14, vsp handlers_universal.go
**Status:** PROPOSED

---

## Problem

User configured two OData services totaling 485 tools. Claude reported "no API available."

**Root cause:** LLMs have practical limits on tool count (~128). Exceeding this causes:
- Context rot (degraded reasoning)
- Tool selection failures
- High token consumption (~14,000 tokens for schemas)

---

## Current Architecture

```
$metadata (XML)
    ↓ parse
ODataMetadata struct
    ↓ generate
N MCP Tools
    ↓ register
MCP Schema (~14,000 tokens for large services)
```

**Tool generation:**
- 1 `odata_service_info` tool (always)
- Per EntitySet: up to 7 tools (filter, count, search, get, create, update, delete)
- Per FunctionImport: 1 tool

**Example calculation (API_BUSINESS_PARTNER):**
```
1 service_info
+ 50 entity sets × 7 operations = 350
+ 72 function imports = 72
= 423 tools (maximum)
```

**Conditionals reduce this:**
- `search` only if `Searchable=true`
- `create/update/delete` only if entity supports it AND not read-only mode
- Operations can be disabled via `--operation-filter`

---

## Proposed Architecture

```
$metadata (XML)
    ↓ parse
ODataMetadata struct
    ↓ generate description
1 MCP Tool + metadata in description (~300 tokens)
    ↓ route internally
Action handlers
```

---

## Universal Tool Design

### Tool Definition

```go
mcp.NewTool("OData",
    mcp.WithDescription(generateDescription(metadata)),
    mcp.WithString("action",
        mcp.Required(),
        mcp.Description("Operation: list|get|create|update|delete|count|call"),
    ),
    mcp.WithString("target",
        mcp.Required(),
        mcp.Description("Entity set name (e.g., 'Products') or function name"),
    ),
    mcp.WithObject("params",
        mcp.Description("Action-specific parameters"),
    ),
)
```

### Description Generator

```go
func generateDescription(meta *models.ODataMetadata) string {
    var sb strings.Builder

    sb.WriteString(fmt.Sprintf("OData service: %s\n\n", meta.ServiceRoot))

    // List entities with capabilities
    sb.WriteString("Entities:\n")
    for name, es := range meta.EntitySets {
        ops := []string{"list", "get"}
        if es.Creatable { ops = append(ops, "create") }
        if es.Updatable { ops = append(ops, "update") }
        if es.Deletable { ops = append(ops, "delete") }
        if es.Searchable { ops = append(ops, "search") }
        sb.WriteString(fmt.Sprintf("  %s [%s]\n", name, strings.Join(ops, ",")))
    }

    // List functions
    if len(meta.FunctionImports) > 0 {
        sb.WriteString("\nFunctions:\n")
        for name, fn := range meta.FunctionImports {
            sb.WriteString(fmt.Sprintf("  %s(%s)\n", name, formatParams(fn)))
        }
    }

    // Usage examples
    sb.WriteString(`
Actions:
  list   - Query entities with filter/select/expand/orderby/top/skip
  get    - Retrieve single entity by key
  create - Create new entity
  update - Update existing entity (method: PUT|PATCH|MERGE)
  delete - Delete entity by key
  count  - Count entities matching filter
  call   - Execute function/action

Examples:
  action="list" target="Products" params={"filter":"Price gt 100","top":10}
  action="get" target="Products" params={"key":{"ID":123}}
  action="create" target="Orders" params={"data":{"CustomerID":"C001"}}
  action="call" target="ReleaseOrder" params={"OrderID":"O001"}
`)

    return sb.String()
}
```

### Generated Description Example

```
OData service: https://api.example.com/sap/opu/odata/sap/API_BUSINESS_PARTNER

Entities:
  A_BusinessPartner [list,get,create,update,delete]
  A_BusinessPartnerAddress [list,get,create,update,delete]
  A_BusinessPartnerBank [list,get,create,update]
  A_BusinessPartnerContact [list,get]
  ...

Functions:
  BlockBusinessPartner(BusinessPartner: string)
  UnblockBusinessPartner(BusinessPartner: string)
  ...

Actions:
  list   - Query entities with filter/select/expand/orderby/top/skip
  get    - Retrieve single entity by key
  ...
```

---

## Internal Routing

```go
func (b *ODataMCPBridge) handleUniversalTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    action := getString(req.Params.Arguments, "action")
    target := getString(req.Params.Arguments, "target")
    params := getObject(req.Params.Arguments, "params")

    // Validate target exists
    entitySet, isEntity := b.metadata.EntitySets[target]
    function, isFunction := b.metadata.FunctionImports[target]

    if !isEntity && !isFunction {
        return nil, fmt.Errorf("unknown target: %s", target)
    }

    // Route to handler
    switch action {
    case "list":
        return b.handleList(ctx, target, params)
    case "get":
        return b.handleGet(ctx, target, params)
    case "create":
        if !entitySet.Creatable {
            return nil, fmt.Errorf("%s does not support create", target)
        }
        return b.handleCreate(ctx, target, params)
    case "update":
        if !entitySet.Updatable {
            return nil, fmt.Errorf("%s does not support update", target)
        }
        return b.handleUpdate(ctx, target, params)
    case "delete":
        if !entitySet.Deletable {
            return nil, fmt.Errorf("%s does not support delete", target)
        }
        return b.handleDelete(ctx, target, params)
    case "count":
        return b.handleCount(ctx, target, params)
    case "call":
        if !isFunction {
            return nil, fmt.Errorf("%s is not a function", target)
        }
        return b.handleFunctionCall(ctx, target, params)
    default:
        return nil, fmt.Errorf("unknown action: %s", action)
    }
}
```

---

## Params Schema by Action

| Action | Params |
|--------|--------|
| `list` | `{filter, select, expand, orderby, top, skip}` |
| `get` | `{key: {field: value, ...}, select, expand}` |
| `create` | `{data: {...}}` |
| `update` | `{key: {...}, data: {...}, method: "PUT\|PATCH\|MERGE"}` |
| `delete` | `{key: {...}}` |
| `count` | `{filter}` |
| `call` | Function-specific parameters |

---

## Comparison

| Metric | Current (485 tools) | Universal (1 tool) |
|--------|--------------------|--------------------|
| Tool count | 485 | 1 |
| Schema tokens | ~14,000 | ~300-500 |
| Context usage | ~22% | ~0.5% |
| Discovery | Implicit (tool names) | Explicit (description) |
| Validation | MCP schema level | Application level |
| Scalability | Limited by LLM | Unlimited |

---

## Migration Path

### Phase 1: Add Universal Mode (non-breaking)

```bash
# Current behavior (default)
odata-mcp --service https://...

# New universal mode
odata-mcp --universal --service https://...
```

### Phase 2: Make Universal Default

```bash
# Universal (default)
odata-mcp --service https://...

# Legacy multi-tool mode
odata-mcp --legacy-tools --service https://...
```

### Phase 3: Deprecate Multi-Tool

Remove `--legacy-tools` after migration period.

---

## Implementation Checklist

- [ ] Add `--universal` flag to config
- [ ] Create `generateUniversalDescription()` function
- [ ] Create `handleUniversalTool()` router
- [ ] Reuse existing handlers (handleList, handleGet, etc.)
- [ ] Add params validation per action
- [ ] Update tests
- [ ] Update documentation

---

## References

- vsp `handlers_universal.go` - Single "SAP" tool pattern
- Writer Engineering - "Context rot" with too many tools
- MCP Best Practices - "MCP servers are not REST API wrappers"
