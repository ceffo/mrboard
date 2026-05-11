# Clean Architecture Guide for Service-Modular Monoliths in Go

This document describes a battle-tested layout for a Go codebase that hosts
multiple business services inside one repository ("microservices-within-a-monolith"),
with each service organized along Clean / Hexagonal Architecture lines. It is
written as a generic guide for agentic LLMs developing new projects in the same
spirit. It does not prescribe specific libraries — the *shape* matters more than
the choice of router, ORM, or mock generator.

The most important property is **not** that the modules can be split into
microservices later. It is that **business logic lives in one place, free of
infrastructure**, and every infrastructure concern (HTTP, SQL, third-party APIs,
queues, feature flags) plugs in through an interface that the business package
itself owns.

A single hypothetical service — let's call it `widgetsvc`, which processes
*widgets* against an external *pricing* provider and persists *reports* — is
used throughout the document for illustration. Every name (`Widget`, `Report`,
`Pricing`, `widgetdal`, …) is a placeholder.

---

## 1. Top-level layout

```
cmd/<binary>/main.go              # tiny: calls Start()
internal/cmd/<binary>/...         # boots logger, env, composition root, server
internal/domain/                  # cross-cutting domain primitives (entities, sentinel errors, generic Service interface)
internal/domain/service/<svc>/    # one package per business service — pure business logic
internal/adapters/<svc>dal/       # one package per adapter — implementations of driven ports
internal/storage/<engine>/        # shared infra primitives: pool, transaction wrapper, type helpers
internal/rest/server/             # generic HTTP server (lifecycle only)
internal/rest/<router>/           # router(s) — wire routes to handlers and middleware
internal/rest/handler/<svc>hdl/   # one package per service of HTTP handlers (transport)
internal/rest/middleware/         # request-scoped concerns (logging, auth, rate-limit, tracing)
pkg/                              # exportable utilities (pure libraries; no domain knowledge)
```

Rules of thumb:

- A package `internal/domain/service/<svc>` is the **owner** of the business
  logic for one bounded concern. It defines its own ports, entities, errors,
  and config.
- A package `internal/adapters/<svc>dal` (or `<x>adpt` for external API
  adapters) is a **driven adapter**: it implements an interface declared by the
  service and translates between domain types and external technology.
- A package `internal/rest/handler/<svc>hdl` is a **driver adapter**: it speaks
  HTTP and calls into the service's driver port.
- The dependency arrows always point *toward* the domain. Business code never
  imports `net/http`, the SQL driver, or third-party SDKs.

## 2. Multiple binaries, one composition root

Each binary under `cmd/` is a thin `main` that calls a `Start()` function in
`internal/cmd/<binary>`. `Start()`:

1. Parses CLI flags (e.g. `--env`, `--local`).
2. Primes environment variables from a file (delegated to a small shared helper).
3. Constructs the logger.
4. Constructs the **composition root** (e.g. `backingservice.New(...)`).
5. Picks the router appropriate to this binary (public API, admin API, internal
   events, …).
6. Wraps the router in a generic HTTP server.
7. Installs signal handlers for graceful shutdown, then closes the composition
   root.

Different binaries (`api`, `admin`, `worker`, `cli`) reuse the **same**
composition-root struct and select which routes/handlers to expose. This is
what makes the monolith *feel* modular: services are decoupled enough that a
new binary is just a new wiring file.

## 3. The composition root

A single package — typically named something like `backingservice` or
`composition` — exports a struct holding every service the application needs,
plus shared infra references (DB pool, cache client, message bus). One
`New(logger)` constructor builds them in a fixed order:

1. Storage clients (DB pool, cache, queue).
2. Driven adapters (DALs and external API clients), each parameterized only by
   what they need from storage and config.
3. Domain services, each constructed by passing the adapters as their port
   interfaces.

```go
widgetDAL, _ := widgetdal.New(pg)
pricingClient, _ := pricingadpt.NewClient(log)
widgetSvc, _ := widgetsvc.New(widgetDAL, pricingClient)
```

This is the **only** place where a concrete adapter type meets a service
constructor. Everywhere else they are seen through their interface.

A `Close(ctx)` method walks the services and shuts them down in reverse, using a
shared `domain.Service` interface (see §6).

### 3.1 When the composition root grows

A single struct holding every service is fine until binaries diverge. Once an
`admin` binary is instantiating ten adapters it never uses, the root needs to
split:

- **Per-binary builders.** Each binary calls something like
  `b := composition.New(log).WithStorage().WithUserSvc().WithWidgetSvc()` and
  pays only for what it mounts. Cheaper boot, smaller blast radius for a
  misconfigured env var, and the binary's required env vars become explicit.
- **Domain-grouped sub-roots.** Group services by bounded context
  (`bs.Identity`, `bs.Billing`) so a binary can ignore whole domains.

Either way, the rule that the root is the *only* place a concrete adapter meets
a service constructor must hold.

## 4. Anatomy of one service package

A service package (`internal/domain/service/<svc>`) has roughly these files:

| File | Purpose |
|---|---|
| `<svc>.go` | Declares the `SVC` driver port, the driven ports (`DAL`, plus any others such as `Pricing`, `Notifier`), the unexported struct that implements `SVC`, and the `New` / `NewWithConfig` constructors. If your mock toolchain uses inline directives, this is where they live; if it uses a central config file, the interfaces here are referenced from it. |
| `entities.go` | Domain types — plain structs used across the public API of the package. No tags from infra packages. |
| `errors.go` | Service-local sentinel errors. (Cross-domain ones live in `internal/domain`.) |
| `config.go` | A `Config` struct populated from environment variables, plus a `Validate()` method. |
| One file per use case (`process_widget.go`, `get_report.go`, …) | The actual business logic, one method on the service struct per file. |
| One `_test.go` per use case | Black-box tests in `package <svc>_test`, using mocks of the driven ports. |
| `mock_<svc>.go` | Generated mocks of the ports (kept in the same package so tests reference them as `<svc>.NewMockDAL`). |

### 4.1 Ports

```go
// Driver port — what the outside calls. Used internally for documenting the
// full surface and for satisfying domain.Service in the composition root.
// Callers should NOT depend on this fat interface (see §7.3).
type SVC interface {
    domain.Service
    ProcessWidget(ctx context.Context, log Logger, widgetID string) (*Report, error)
    GetReport(ctx context.Context, log Logger, widgetID string) (*Report, error)
    // ... one method per use case
}

// Driven ports — what the service needs from the outside.
type DAL interface {
    GetWidget(ctx context.Context, widgetID string) (*Widget, error)
    StoreReport(ctx context.Context, widgetID string, r *Report) error
    WrapTx(ctx context.Context, log Logger, body func(txDAL DAL) error) error
    // ...
}

type Pricing interface {
    Quote(ctx context.Context, w *Widget) (*Report, error)
}

// Async fan-out is also a port. Don't let "the eventbus" be a global the
// service reaches for.
type Publisher interface {
    Publish(ctx context.Context, evt Event) error
}
```

Key conventions:

- All ports live **in the service package**. Adapters import the service package
  to satisfy these interfaces — never the reverse.
- Ports speak in **domain types** (`Widget`, `Report`), not storage/transport
  types.
- The driver port embeds a small `domain.Service` interface so every service
  has uniform `Health` / `Close` methods.
- **Async I/O is a port too.** Publishers, subscribers, schedulers, and
  workqueues are first-class driven ports on the same footing as `DAL`. If a
  use case ends with "and emit an event", that emission is an interface call,
  not a hidden global.
- **Beware the fat `SVC`.** The service implementation exposes every use case,
  but consumers (handlers, jobs, other services) should depend on **narrow
  per-use-case interfaces** declared at the consumer site (§7.3). The fat
  interface is for the composition root and reflective tooling; it is not what
  callers should type against.

### 4.2 Constructor

```go
func New(dal DAL, pricing Pricing) (SVC, error) {
    cfg := &Config{}
    if err := envparse(cfg); err != nil { return nil, err }
    return NewWithConfig(cfg, dal, pricing)
}

func NewWithConfig(cfg *Config, dal DAL, pricing Pricing) (SVC, error) {
    if cfg == nil || dal == nil || pricing == nil { /* nil-checks */ }
    if err := cfg.Validate(); err != nil { return nil, err }
    return &service{config: cfg, dal: dal, pricing: pricing}, nil
}
```

- `New` is what the composition root calls — environment-driven.
- `NewWithConfig` is what tests call — pure dependency injection.
- The struct (`service`) is unexported. Outside callers know it only as `SVC`.

### 4.3 Use-case method shape

```go
func (s *service) ProcessWidget(ctx context.Context, log Logger, widgetID string) (*Report, error) {
    widget, err := s.dal.GetWidget(ctx, widgetID)
    if err != nil { return nil, err }

    report, err := s.pricing.Quote(ctx, widget)
    if err != nil { return nil, err }
    report.Timestamp = time.Now()

    if report.Status == StatusFlagged {
        if err := s.dal.StoreReport(ctx, widget.ID, report); err != nil {
            return nil, err
        }
    }
    return report, nil
}
```

A use case orchestrates ports. It contains the business rules ("only persist
flagged reports") but no SQL, no HTTP, no JSON.

### 4.4 Tests of the service

```go
package widgetsvc_test

func Test_service_ProcessWidget(t *testing.T) {
    ctrl := newMockController(t)
    mdal := widgetsvc.NewMockDAL(ctrl)
    mpricing := widgetsvc.NewMockPricing(ctrl)
    s, _ := widgetsvc.NewWithConfig(&widgetsvc.Config{ /* ... */ }, mdal, mpricing)

    // arrange expectations on mdal / mpricing, act, assert
}
```

- Tests live in an external `_test` package — they can only see the public API.
- Every dependency is a mock of a port the service itself owns. There is never
  a mock of an adapter or of HTTP. This is the practical payoff of putting ports
  inside the service package.

### 4.5 Calling another service

When `widgetsvc` needs something from `usersvc`, the wrong answers are: import
`usersvc` directly (the dependency graph now has a cycle waiting to happen), or
reach into `usersvc`'s DAL (you've smuggled persistence across a boundary).

The right answer is the same as for any external dependency — declare a port:

```go
// in widgetsvc
type Users interface {
    LookupOwner(ctx context.Context, userID string) (*Owner, error)
}
```

`usersvc.SVC` already satisfies this method (or you write a thin adapter that
narrows it). The composition root binds them:

```go
widgetSvc, _ := widgetsvc.New(widgetDAL, pricingClient, userSvc /* implements widgetsvc.Users */)
```

This keeps `widgetsvc`'s test suite mock-only (no need to spin up `usersvc`),
and prevents a refactor of `usersvc`'s public API from cascading into
`widgetsvc` — only the narrow `Users` interface matters.

## 5. Anatomy of one adapter (DAL)

A DAL package (`internal/adapters/<svc>dal`) has roughly these files:

| File | Purpose |
|---|---|
| `<svc>dal.go` | Defines the adapter struct (e.g. `WidgetDAL`), `New(db) (*WidgetDAL, error)`, and `WrapTx`. May host a `//go:generate` directive for the SQL-codegen mocks, or be referenced from a central mock-config file. |
| One file per port method (`get_widget.go`, `store_report.go`, …) | Translates domain inputs into storage calls and storage rows back into domain types. |
| One `_test.go` per file | White-box tests (`package <svc>dal`) using mocks of the **codegen-generated** querier. |
| `queries/*.sql` | Hand-written SQL, annotated with codegen directives (e.g. `-- name: FindWidget :one`, parameter markers). |
| `queries/*.sql.go` | Code generated from the SQL files — typed `Querier` interface, `*Params` and `*Row` structs. **Do not edit.** |
| `queries/mock_querier.sql.go` | Generated mock of the codegen `Querier`, used by the DAL tests. |

### 5.1 Adapter shape

```go
type WidgetDAL struct{ db storage.Querier }

func New(db storage.Querier) (*WidgetDAL, error) { return &WidgetDAL{db: db}, nil }

func (w *WidgetDAL) GetWidget(ctx context.Context, widgetID string) (*widgetsvc.Widget, error) {
    q := queries.NewQuerier(w.db)
    return getWidget(ctx, q, widgetID)
}

func getWidget(ctx context.Context, q queries.Querier, widgetID string) (*widgetsvc.Widget, error) {
    row, err := q.FindWidget(ctx, widgetID)
    if errors.Is(err, storage.ErrNoRows) { return nil, domain.ErrWidgetNotFound }
    if err != nil { return nil, err }
    // map row → domain.Widget
}
```

Conventions:

- The exported method takes the storage handle from the struct, builds a fresh
  `queries.Querier`, and delegates to a private function that takes the
  `Querier` directly. That private function is what tests target — they pass in
  the generated mock.
- Storage-specific errors (the driver's not-found sentinel, unique-violation
  codes, …) are translated into **domain sentinel errors** at this boundary.
  The service only ever sees `domain.ErrWidgetNotFound`, never the
  driver-specific value.
- Domain types are constructed here. The service package's structs never carry
  storage-specific tags or driver-typed fields.

### 5.2 SQL codegen

SQL queries are kept as plain `.sql` files; a generator compiles them into a
typed `Querier` interface at build time. The specific tool is interchangeable —
what matters is:

- The generated artefacts live in a `queries/` subpackage of the DAL, alongside
  their source `.sql` files.
- The DAL never builds SQL strings by hand outside of the generator.
- The generated `Querier` is mockable, so DAL tests can assert on the exact
  parameters passed to each query.
- Migrations are run before generation so the generator sees current schema
  types. A small wrapper script (e.g. under `scripts/`) discovers each
  `queries/` directory and invokes the generator with the live DB.

A classical hand-written DAL with `database/sql` queries is also fine — the
layering is what carries the architecture, not the codegen.

### 5.3 Transactions

Two-level pattern:

```go
// storage layer
type Querier interface {
    Query(...); QueryRow(...); Exec(...); SendBatch(...)
    WrapTx(ctx context.Context, log Logger, body func(Querier) error) error
}

// DAL layer
func (w *WidgetDAL) WrapTx(ctx context.Context, log Logger, body func(widgetsvc.DAL) error) error {
    return w.db.WrapTx(ctx, log, func(q storage.Querier) error {
        txDAL, err := New(q)
        if err != nil { return err }
        return body(txDAL)
    })
}
```

- The storage layer owns rollback/commit semantics and is the only thing that
  knows about driver-level transactions.
- The DAL exposes its own `WrapTx` that returns a *DAL bound to the
  transaction*, so the service can drive transactions in business terms:

```go
err := s.dal.WrapTx(ctx, log, func(txDAL DAL) error {
    if err := txDAL.StoreEvent(ctx, ev); err != nil { return err }
    return txDAL.StoreReport(ctx, widgetID, report)
})
```

- The service still doesn't import the SQL driver. The transaction is just a
  function with a different DAL passed to it.

### 5.4 Read paths can skip the domain detour

Forcing every read through `row → domain entity → JSON` pays the mapping cost
twice on hot endpoints that just want to render data. For high-traffic reads,
expose a separate **read port** on the service that returns a flat view struct,
with a DAL implementation that goes straight from generated rows to the view:

```go
// in widgetsvc — separate from DAL
type Reader interface {
    WidgetSummary(ctx context.Context, widgetID string) (*WidgetSummaryView, error)
}
```

Writes still flow through the domain model and `DAL`. Reads that don't need
business rules take the short path. This is a lightweight CQRS split — no
event sourcing, no separate database, just a different port.

Don't reach for this on day one. Reach for it when a profiler points at the
mapping code.

## 6. Cross-cutting domain primitives

`internal/domain/` contains things shared by *every* service:

- A generic `Service` interface (`Health`, `Close`) embedded by every service's
  driver port. The composition root iterates a `[]domain.Service` to shut
  everything down.
- Sentinel errors used across the system. `errors.Is(err, domain.ErrXxx)` works
  at any layer because the DAL, the service, and the handler all agree on the
  same value.
- Other generic helpers (cipher primitives, retryable-error wrapping, common
  events). Anything specific to one service goes inside that service's package
  instead.

## 7. The HTTP layer

### 7.1 Server

`internal/rest/server` is a tiny package with `New(handler) (*Server, error)`
and `ListenAndServe / Shutdown`. It does nothing application-specific. Its
config (port, address) is parsed from env in the same way services parse
theirs.

### 7.2 Routers

`internal/rest/<router>/router.go` builds a router and **delegates route
registration** to each handler package — it does not list every endpoint
itself. The router file's job is to assemble middleware stacks and call
`RegisterRoutes` in order:

```go
func New(log Logger, bs *backingservice.BackingServices) (Router, error) {
    r := newRouter()
    r.Use(middleware.RequestLog, middleware.Trace, middleware.MapDomainErrors)

    v1 := r.Subrouter("/v1")
    v1.Use(authMiddleware(bs.UserService, log))

    widgethdl.RegisterRoutes(v1, log, bs.WidgetService)
    userhdl.RegisterRoutes(v1, log, bs.UserService)
    // ...
    return r, nil
}
```

This keeps each service's URL surface co-located with its handlers, eliminates
merge-conflict hotspots in the router file as the API grows, and makes it
trivial for a binary to mount only the slices it cares about.

Each binary mounts its own router (public API, admin API, event ingest, …)
against the same composition root. This is where the
"microservices-in-a-monolith" shape becomes visible: a binary chooses which
slice of the API to expose.

### 7.3 Handlers

A handler package owns its routes and its dependencies. Crucially, handlers
**do not depend on the fat `SVC` interface** — they declare a narrow,
per-handler interface and accept that:

```go
package widgethdl

// Narrow consumer-side interface. Reads only what this handler calls.
type reportGetter interface {
    GetReport(ctx context.Context, log Logger, widgetID string) (*widgetsvc.Report, error)
}

func GetReport(log Logger, svc reportGetter) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        widgetID, ok := pathVar(r, "widgetID")
        if !ok { render.BadRequest(ctx, log, w, fmt.Errorf("missing widgetID")); return }

        report, err := svc.GetReport(ctx, log, widgetID)
        if err != nil { render.Error(ctx, log, w, err); return } // see §7.5
        render.JSON(ctx, log, w, report, http.StatusOK)
    }
}

func RegisterRoutes(r Router, log Logger, svc widgetsvc.SVC) {
    r.Handle("GET", "/widget/{widgetID}", GetReport(log, svc))
    // ... other endpoints
}
```

Conventions:

- A handler is a closure built by a top-level `Func(deps...) http.HandlerFunc`.
- The closure's interface parameter is **a tiny interface declared in the
  handler package**, listing only the methods this one handler calls. The
  service struct satisfies it implicitly. This is the Go idiom (accept
  interfaces, return structs) applied at the transport boundary.
- Handlers do three things: parse input, call the service, render output.
- Rendering helpers (`render.JSON`, `render.Error`, …) live in `pkg/` so they
  can be reused across handler packages.
- Handlers are tested with a one-method fake of their narrow interface — no
  need to mock the entire service.

### 7.4 Middleware

Cross-cutting HTTP concerns live in `internal/rest/middleware/`: structured
request logging, distributed tracing, authn, rate limiting, request body
parsing for events. Middleware is composed onto routers, never embedded in
handlers.

### 7.5 Centralized error mapping

Mapping `domain.Err*` to HTTP statuses is policy that should live in **one
place**, not be repeated in every handler's `switch errors.Is(...)` ladder. Two
common shapes:

- **A render helper that knows the mapping table.** `render.Error(ctx, log, w,
  err)` inspects the error with `errors.Is` against a registry of
  `(domain.Err*, httpStatus)` pairs and writes the appropriate response. New
  domain errors register themselves once.
- **A middleware that catches errors returned from handlers.** Handlers return
  `error`; a wrapping middleware translates and renders. Requires a thin
  `httpHandlerE` shape but eliminates the boilerplate entirely.

Either way, when a new sentinel is added to `internal/domain`, only the mapping
table changes — handlers stay untouched.

## 8. Configuration

Each package that needs configuration ships its own `Config` struct:

```go
type Config struct {
    PricingURL  string `env:"WIDGET_PRICING_URL"`
    LocalSource string `env:"WIDGET_LOCAL_SOURCE"`
}

func (c *Config) Validate() error { /* declarative rules */ }
```

- An env-tag library parses env into the struct.
- `Validate()` enforces invariants (mutually exclusive fields, required when
  another is empty, etc.).
- Each `New(...)` parses env, then calls `NewWithConfig(cfg, deps...)` so tests
  can pass a literal config.
- No global config object. Each package's config is private to it.

## 9. Logging

Two valid styles, pick one and be consistent:

- **Logger as an explicit parameter** on every method. Handlers decorate a base
  logger with request-scoped fields (`WithRequestID`, `WithUserID`); services
  and DALs take a `Logger`. Pro: visible in every signature, impossible to
  forget in a background goroutine, easy to mock. Con: signature noise, every
  business method now has a non-business parameter.
- **Logger in `context`**, extracted via a helper (e.g. `slog.FromContext(ctx)`).
  Pro: signatures stay clean. Con: invisible coupling, easy to lose when work
  hops to a goroutine that doesn't carry the same `ctx`.

Both work. The architecture survives either choice; what kills it is mixing
both in the same codebase.

## 10. Mocks

Two equally valid styles for declaring what to mock:

- **Per-interface `//go:generate` directive**, sitting next to the interface
  declaration. `go generate ./...` walks the tree and regenerates each one in
  place. Pro: the source of truth is local — you see at a glance that an
  interface is mocked. Con: scattered directives, easy to drift between
  services, and every interface that wants a mock needs its own line.
- **A central config file** (e.g. `.mockery.yaml` at the repo root) listing all
  interfaces, their packages, output locations, and naming. One command
  regenerates everything. This is the direction modern generators (mockery v2+)
  push, and it's the better default for a multi-service repo: uniform output,
  one place to change conventions, no `//go:generate` lines to maintain. Pro:
  consistency, easy onboarding, refactor-friendly. Con: the indirection means
  you check a YAML file rather than the source to know what's mocked.

Two equally valid placements for the generated mocks:

- **Same package** as the interface (e.g. `mock_widgetsvc.go` in `widgetsvc/`).
  External `_test` packages spell them as `widgetsvc.NewMockDAL(ctrl)`.
  Trade-off: the mock links into production binaries unless build tags or
  `_test.go` naming keep it out.
- **Sibling `mocks/` package** (e.g. `widgetsvc/mocks/`). Mocks never link into
  production; tests import them explicitly. Trade-off: one extra import per
  test file. This is mockery's default when driven by a config file.

For codegen-generated DAL queriers (the SQL `Querier` interface), the same
choice applies, but in practice the mock lives next to the generated code in
the `queries/` subpackage — that subpackage is already a generated artefact
boundary.

A repo-level make target (e.g. `make generate`) drives whichever toolchain
you've picked.

The choice of generator, declaration style, and placement is not load-bearing.
What **is** load-bearing: the interfaces being mocked are owned by the business
package, and the consumer (handler, other service) types against a narrow
interface that's trivial to mock either way.

## 11. Concrete vertical slice — what to look at first

To understand any single feature, walk the slice from outside in:

1. **Route**: find the path in `internal/rest/<router>/router.go`. It points at
   a constructor function in a handler package.
2. **Handler**: `internal/rest/handler/<svc>hdl/<usecase>.go` — see what input
   it parses, which `SVC` method it calls, and how it maps errors to HTTP.
3. **Service interface**: `internal/domain/service/<svc>/<svc>.go` — find the
   method on `SVC`. Note its driven ports.
4. **Use case**: `internal/domain/service/<svc>/<usecase>.go` — the business
   logic. This is the file you would read to a domain expert.
5. **Adapter**: `internal/adapters/<svc>dal/<usecase>.go` — see how each port
   call lands in storage. Note the error translation back to `domain.Err*`.
6. **Codegen / SQL**: `internal/adapters/<svc>dal/queries/<file>.sql` — the
   actual queries. The `.sql.go` siblings are generated artefacts.

When adding a new feature, walk the slice in the **opposite direction**:
write the SQL or external call first, expose it on the relevant port, implement
the use case in the service, write the handler, attach the route. The
composition root almost never has to change unless an entirely new service is
added.

## 12. Checklist when introducing a new service

For a hypothetical `widgetsvc`:

- [ ] `internal/domain/service/widgetsvc/`
  - [ ] `widgetsvc.go` with `SVC` (embeds `domain.Service`), driven ports,
        struct, `New` and `NewWithConfig`. Register the ports with the project's
        mock toolchain (inline `//go:generate` or central mock-config entry).
  - [ ] `entities.go`, `errors.go`, `config.go`.
  - [ ] One file per use case + matching `_test.go` in `package widgetsvc_test`.
- [ ] `internal/adapters/widgetdal/` (and one package per additional adapter the
      service needs, e.g. `pricingadpt/`)
  - [ ] `widgetdal.go` with struct, `New`, `WrapTx`.
  - [ ] One file per port method, with white-box test.
  - [ ] `queries/*.sql` and codegen output.
- [ ] `internal/rest/handler/widgethdl/`
  - [ ] One file per HTTP endpoint exposing a `func(deps) http.HandlerFunc`.
  - [ ] Tests using a mock of `widgetsvc.SVC`.
- [ ] Wire it in the composition-root package:
  - [ ] Construct the adapter(s).
  - [ ] Construct the service via `widgetsvc.New(adapters...)`.
  - [ ] Add the field to the composition-root struct.
  - [ ] Add the service to the `Close` slice.
- [ ] Mount handlers on the appropriate router(s).
- [ ] Add env vars to local config / deployment manifests.
- [ ] Run the codegen + mock targets.

## 13. Things to keep out of business code

If you find yourself doing any of these inside a `domain/service/` package, stop
and push it to an adapter:

- Importing `net/http`, `database/sql`, a SQL driver, a third-party SDK, a
  message bus client, a feature-flag client, a tracer.
- Reading environment variables outside of the package's own `Config`.
- Constructing storage rows or HTTP requests by hand.
- Catching infra-specific error types (driver-specific not-found sentinels, HTTP
  4xx codes). Those are mapped to domain errors at the adapter boundary.
- Calling `time.Sleep`, spawning goroutines that outlive a request, or touching
  the file system. (If the use case truly needs them, expose them as a port and
  inject them.)

Conversely, what an adapter must **not** do:

- Hold business rules. Decisions like "we only persist flagged reports" belong
  in the service. The DAL just stores what it is told.
- Return storage-native types across the package boundary. Always map to the
  service's domain types before returning.
- Depend on another service or its DAL. If two services need the same data,
  give each one its own port and let the composition root wire them up.

---

### TL;DR

- **One package per service**, owning its own ports, entities, config, errors,
  and tests.
- **One package per adapter**, importing the service to satisfy a port.
- **One composition root** that knows every concrete type and wires services to
  adapters.
- **One thin binary** per deployable surface, all sharing the same composition
  root and selecting which routes to expose.
- **Codegen** (SQL, mocks) is a build-time tool that lives next to the things it
  generates. The choice of tool is replaceable; the layering is not.
- **The dependency graph always points inward**: HTTP → service interface →
  business logic → port interface → adapter → infrastructure. Reverse the arrow
  anywhere and you've broken the architecture.
