# Origin, source, and immutability: cross-asset analysis

This document analyzes how the `origin` field, the `source` annotation, and the `createAsUiAsset` query parameter interact to control asset mutability across all asset types in the Dash0 backend.
The goal is to identify inconsistencies that should be standardized.

## Concepts

Three related but distinct mechanisms control whether an asset is editable in the UI:

| Concept | What it is | Where it lives | What it controls |
|---------|-----------|----------------|-----------------|
| **Origin** | A string identifier stored in the database when an asset is created programmatically | DB column (`origin`) or label (`dash0.com/origin`) | Whether `permittedActions` includes `write` for admins |
| **Source** | A derived annotation computed from origin at read time | Annotation (`dash0.com/source`) on dashboards; label (`dash0.com/source`) on views | Additional UI restrictions beyond permissions (e.g., folder moves for dashboards) |
| **`createAsUiAsset`** | A query parameter on POST endpoints that controls whether the asset is treated as UI-created; available on all four asset types (dashboards, views, check rules, synthetic checks) but not on notification channels; only on POST, not PUT or DELETE | Request parameter, not persisted | Whether origin gets an `"api-"` prefix; whether admin write permission is granted at creation |

### How origin controls immutability

All asset types determine `permittedActions` based on origin:
- **Origin is empty/nil** → admin gets `read + write + delete` → **editable** in UI.
- **Origin is non-empty** → admin gets `read + delete` (no `write`) → **immutable** in UI.

### How origin is stored (the UUID gate)

All asset types share the same database-layer pattern when creating a new asset:

```go
id, err := uuid.Parse(originOrId)
isUuid := err == nil
if !isUuid {
    origin = &originOrId   // stored → immutable
}
// if isUuid → origin stays nil → editable
```

This means the **format** of the `originOrId` value silently determines mutability:
- Valid UUID → origin not stored → editable.
- Anything else (including `"api-<uuid>"`) → origin stored → immutable.

## Per-asset analysis

### Dashboards

#### Origin generation

| Code path | `originOrId` value | Origin stored? |
|-----------|-------------------|----------------|
| POST without `createAsUiAsset` | `"api-<uuid>"` via `generateAPIDashboardOriginOrId()` | Yes |
| POST with `createAsUiAsset=true` | bare UUID | No |
| POST without `createAsUiAsset`, YAML has `Dash0Extensions.Id` | value of `Dash0Extensions.Id` (overrides `"api-"` prefix) | Depends on format |
| PUT (any) | URL path parameter as-is | Depends on format |

#### Permission logic

Set at the **route layer** (`dashboards.go:390–396`) based on `createAsUiAsset`:

```go
adminActions := []DashboardAction{DashboardsRead, DashboardsDelete}
if createAsUiAsset {
    adminActions = append(adminActions, DashboardsWrite)
}
```

Members always get `DashboardsRead` only.

For PUT (both machine tokens and user tokens), `createAsUiAsset` is hardcoded to `false` (`dashboards_api.go:353`, `dashboards.go:695`).
This means updating an existing dashboard via PUT always sets admin permissions to read + delete only, even if the original dashboard was UI-created.
**This is a bug: PUT overwrites existing permissions with restricted defaults.**

#### Source annotation

Dashboards have a **derived** `dash0.com/source` annotation, computed from origin at read time by `getDashboardSource()` (`dashboard.go:2220–2233`):

| Origin value | Source | UI effect |
|-------------|--------|-----------|
| nil / empty | `"ui"` | Fully editable, can move to folders |
| Starts with `"tf_"` | `"terraform"` | Cannot move to folders |
| Starts with `"dash0-operator_"` | `"operator"` | Cannot move to folders |
| Anything else | `"api"` | Cannot move to folders |

The source annotation controls **folder operations** in the frontend (`dashboard-list/utils.ts:99–121`), which is separate from the `permittedActions` check that controls **editing** (`use-can-edit-dashboard.ts:42`).

Both checks must pass for full editability: the dashboard needs `dashboards:write` in `permittedActions` AND source must not be `"api"`, `"terraform"`, or `"operator"` for folder operations.

#### Files

- Route layer: `control-plane-api/internal/routes/dashboards.go` (lines 279–438), `dashboards_api.go` (lines 180–365)
- DB layer: `control-plane-api/internal/database/dashboard.go` (lines 1359–1456, 2078–2233)
- Frontend: `ui/src/dashboarding/pages/dashboard-viewer/use-can-edit-dashboard.ts`, `ui/src/dashboarding/pages/dashboard-list/utils.ts`

---

### Check rules

#### Origin generation

| Code path | `originOrId` value | Origin stored? |
|-----------|-------------------|----------------|
| POST (machine token), no ID in body | `"api-<uuid>"` | Yes |
| POST (machine token), ID in body | `"api-<id>"` | Yes (unless ID is a UUID, then `"api-<uuid>"` fails UUID parse regardless) |
| POST (user token), no ID in body | `"api-<uuid>"` but `createAsUiAsset` auto-set to `true` locally (see note) | Yes |
| POST with `createAsUiAsset=true` | bare UUID | No |
| PUT (any) | URL path parameter as-is | Depends on format |

**Note on auto-set `createAsUiAsset`**: In the public API user-token path (`checkrules_api.go:172–174`), when no ID is provided, the local variable `createAsUiAsset` is set to `true`.
However, the `if` condition on line 176 checks `request.Params.CreateAsUiAsset` (the request parameter), not the local variable.
Since the request parameter is nil, the `"api-"` prefix is **still applied**.
The local `createAsUiAsset=true` is then passed to `putOrganizationCheckRuleInternal` as the `isUpdate` parameter, where it is used only for validation logic—not for permission or origin decisions.
This means the auto-set has **no effect on origin or permissions** for user tokens.

#### Permission logic

Set at the **database layer** (`checkrule.go:2480–2512`) based on `origin != nil`:

```go
checkRuleCreatedViaAPI := origin != nil && *origin != ""
if auth.ClerkOrganizationRole == AdminRole {
    if !checkRuleCreatedViaAPI {
        permittedActions[CheckRuleWrite] = true
    }
    permittedActions[CheckRuleDelete] = true
}
```

**Architectural difference from dashboards/views**: permissions are not set at the route layer via `createAsUiAsset`; they are computed at read time from the stored origin.
This means check rules cannot have their permissions incorrectly overwritten by PUT (unlike dashboards/views), but it also means the only way to make a check rule editable is to ensure origin is nil.

#### Source annotation

Check rules do **not** have a `dash0.com/source` annotation.
Immutability is determined solely by `permittedActions` in the UI (`use-can-edit-check-rule.tsx`).
The frontend uses `isCreatedByAPI(checkRule)` which checks `checkRule.origin != null`.

#### Files

- Route layer: `control-plane-api/internal/routes/checkrules.go` (lines 707–918), `checkrules_api.go` (lines 103–330)
- DB layer: `control-plane-api/internal/database/checkrule.go` (lines 1554–1614, 2480–2530)
- Frontend: `ui/src/alerting/utils/use-can-edit-check-rule.tsx`, `ui/src/alerting/pages/check-rule-list-page/components/check-rule-title.tsx`

---

### Views

#### Origin generation

| Code path | `originOrId` value | Origin stored? |
|-----------|-------------------|----------------|
| POST without `createAsUiAsset` | `"api-<uuid>"` (or override from `Dash0Comorigin` label) | Yes |
| POST with `createAsUiAsset=true` | bare UUID | No |
| PUT (any) | URL path parameter as-is | Depends on format |

#### Permission logic

Set at the **route layer** (`views.go:590–621`) based on `createAsUiAsset`, **identical pattern to dashboards**:

```go
adminActions := []ViewAction{ViewsRead, ViewsDelete}
if createAsUiAsset {
    adminActions = append(adminActions, ViewsWrite)
}
```

For PUT, `createAsUiAsset` is hardcoded to `false` (`views_api.go:357`, `views.go:458`).
**Same bug as dashboards: PUT overwrites permissions with restricted defaults.**

#### Source annotation

Views have a `dash0.com/source` **label** (not annotation) with three values:

| Value | Meaning |
|-------|---------|
| `"builtin"` | System-provided default views |
| `"external"` | Created via API with non-UUID origin |
| `"userdefined"` | Created by users in UI |

The source label is set:
- To `"external"` by the database layer on update when `originOrId` is not a UUID (`view.go:1532–1533`).
- By the request body when the caller provides it (validated to be `"userdefined"` or `"external"` only; `views.go:485`).

#### Files

- Route layer: `control-plane-api/internal/routes/views.go` (lines 470–641), `views_api.go` (lines 181–369)
- DB layer: `control-plane-api/internal/database/view.go` (lines 1455–1534)
- Utility: `control-plane-api/internal/utils/view.go` (lines 64–75)

---

### Synthetic checks

#### Origin generation

| Code path | `originOrId` value | Origin stored? |
|-----------|-------------------|----------------|
| POST without `createAsUiAsset` | `"api-<uuid>"` (or override from `Dash0Comorigin` label) | Yes |
| POST with `createAsUiAsset=true` | bare UUID | No |
| PUT (any) | URL path parameter as-is | Depends on format |

#### Permission logic

Set at the **database layer** (`synthetic_check.go:2264–2296`) based on `origin != nil`, **identical pattern to check rules**:

```go
syntheticCheckCreatedViaAPI := labels.Dash0Comorigin != nil && *labels.Dash0Comorigin != ""
if auth.ClerkOrganizationRole == AdminRole {
    if !syntheticCheckCreatedViaAPI {
        permittedActions[SyntheticCheckWrite] = true
    }
    permittedActions[SyntheticCheckDelete] = true
}
```

**Architectural difference from dashboards/views**: same as check rules — permissions computed at read time from origin, not set at creation time via `createAsUiAsset`.

#### Source annotation

Synthetic checks do **not** have a `dash0.com/source` annotation.
Immutability is determined solely by `permittedActions` and `isCreatedByAPI()` in the frontend (`synthetics/pages/synthetics-detail-page/utils/is-created-by-api.ts`).

#### Files

- Route layer: `control-plane-api/internal/routes/synthetic_checks.go` (lines 690–964), `synthetic_checks_api.go` (lines 104–337)
- DB layer: `control-plane-api/internal/database/synthetic_check.go` (lines 1295–1461, 2264–2321)
- Frontend: `ui/src/synthetics/pages/synthetics-detail-page/utils/is-created-by-api.ts`

---

### Notification channels

Notification channels do **not** participate in the origin/immutability system.
They are the only asset type managed by the CLI where origin has no effect on editability.

#### Origin handling

- Origin is stored directly from the request body (`dash0.com/origin` label), not auto-generated by the backend.
- There is no `createAsUiAsset` parameter (not in the OpenAPI spec, not in the route handlers).
- There is no `"api-"` prefix logic.
- UUID collision validation exists (`validateNotificationChannelOrigin`) but no `uuid.Parse` gate for deciding whether to store origin.

#### Permission logic

Simple admin-role check: only admins can create, update, or delete notification channels (`requireAdminRole()` in the public API, `CheckUserAuthAndExpectAdminRoleInOrganization()` in the internal API).
There are no fine-grained `permittedActions`, no `isActionable` checks, and no origin-based write guards — in the backend, the database layer, or the frontend.
Any admin can edit or delete any notification channel regardless of whether it has an origin.

#### Cannot be made read-only

Unlike the other four asset types, notification channels **cannot** be made immutable.
There is no mechanism — origin-based, permission-based, or otherwise — to prevent an admin from modifying a notification channel.
The frontend does not render `ReadOnlyTag` or disable edit/delete buttons based on origin.

#### Source annotation

No `dash0.com/source` annotation.

#### Files

- Route layer: `control-plane-api/internal/routes/notificationchannels.go`, `notificationchannels_api.go`
- DB layer: `control-plane-api/internal/database/notificationchannel.go`
- Service layer: `control-plane-api/internal/service/notification_channel_service/`

---

## Summary of inconsistencies

### 1. Permission logic lives in two different places

| Asset type | Where admin write permission is determined |
|-----------|-------------------------------------------|
| **Dashboards** | Route layer, at creation/update time, based on `createAsUiAsset` |
| **Views** | Route layer, at creation/update time, based on `createAsUiAsset` |
| **Check rules** | Database layer, at read time, based on `origin != nil` |
| **Synthetic checks** | Database layer, at read time, based on `origin != nil` |

**Impact**: For dashboards and views, PUT always sets `createAsUiAsset=false`, which overwrites permissions to read+delete even on existing editable assets.
For check rules and synthetic checks, permissions are always computed from the current origin value, so they cannot be accidentally overwritten.

### 2. The `source` annotation is not standardized

| Asset type | Has source annotation? | Values | Derived from origin? |
|-----------|----------------------|--------|---------------------|
| **Dashboards** | Yes (`dash0.com/source` annotation) | `ui`, `api`, `terraform`, `operator` | Yes, at read time via `getDashboardSource()` |
| **Views** | Yes (`dash0.com/source` label) | `builtin`, `external`, `userdefined` | Partially (set to `"external"` on update when origin is non-UUID) |
| **Check rules** | No | — | — |
| **Synthetic checks** | No | — | — |

The enum values are completely different between dashboards and views, and two asset types lack the annotation entirely.

### 3. The `uuid.Parse` gate creates format-dependent behavior

All four asset types use the same `uuid.Parse(originOrId)` check in the database layer to decide whether to store origin.
This means the **format** of a user-provided ID silently determines mutability:

| ID format | Origin stored? | Asset editable in UI? |
|-----------|---------------|----------------------|
| `"a1b2c3d4-5678-90ab-cdef-1234567890ab"` (UUID) | No | Yes |
| `"prod-overview"` (slug) | Yes | No |
| `"api-a1b2c3d4-..."` (auto-generated by POST) | Yes | No |

There is no documentation or user-facing signal that explains this behavior.

### 4. The CLI never passes `createAsUiAsset`

The Go API client (`dash0-api-client-go`) does not expose the `createAsUiAsset` query parameter on any endpoint.
As a result:
- All CLI-created assets via POST get an `"api-"` prefixed origin → always immutable.
- CLI-created assets via PUT with a UUID ID → origin not stored → editable.
- CLI-created assets via PUT with a slug ID → origin stored → immutable.

### 5. PUT hardcodes `createAsUiAsset=false` for dashboards and views

Both the public API PUT and the internal PUT handlers pass `createAsUiAsset=false`:
- `dashboards_api.go:353`, `dashboards.go:695`
- `views_api.go:357`, `views.go:458`

This means every PUT (update) operation on dashboards and views resets admin permissions to read+delete only.
If a dashboard was originally created with `createAsUiAsset=true` (editable), updating it via PUT makes it immutable.

### 6. Check rules auto-set `createAsUiAsset` in user-token path (but it has no effect)

In the public API user-token path for check rules (`checkrules_api.go:172–174`), the local variable `createAsUiAsset` is set to `true` when no ID is provided.
But the `"api-"` prefix is still applied because the condition checks `request.Params.CreateAsUiAsset` (nil), not the local variable.
The local variable is then passed as `isUpdate` to the internal handler, where it is only used for validation — not for permission decisions.
This is confusing code that has no practical effect.

### 7. Notification channels have no immutability mechanism

The other four asset types all support making assets immutable via origin.
Notification channels do not — origin is just an external identifier for upsert semantics.
Any admin can edit or delete any notification channel, regardless of who or what created it.
If notification channels should be manageable as code (like the other asset types), they need the same immutability guarantees to prevent UI users from accidentally overwriting programmatically-managed configurations.

## Recommendations

1. **Standardize where permission logic lives**: either always at the route layer (like dashboards/views) or always at the database layer (like check rules/synthetic checks), not a mix.

2. **Standardize the `source` annotation**: either all asset types have it with a common enum, or none do. The current state (dashboards and views with different enums, check rules and synthetic checks without) is confusing.

3. **Decouple mutability from ID format**: the `uuid.Parse` gate should not be the mechanism that determines whether an asset is externally managed. Use an explicit signal (e.g., a `managed-by` annotation or the `createAsUiAsset` parameter).

4. **Fix the PUT permission overwrite bug**: PUT should not reset admin permissions when updating an existing asset. Either preserve existing permissions or compute them from the current origin (like check rules and synthetic checks do).

5. **Expose `createAsUiAsset` in the API client**: the CLI should be able to create assets that are editable in the UI, just like the Dash0 UI itself does.

6. **Clean up the dead auto-set in check rules**: the `createAsUiAsset = true` assignment in `checkrules_api.go:174` has no practical effect and should be removed or fixed to actually work.
