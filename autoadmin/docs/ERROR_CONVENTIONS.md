# Error conventions

autoadmin uses three error layers.

## 1. Error mechanism

`internal/shared/apperror/error.go` defines the immutable `Error` type, accessors, `errors.As` support and `WithCause`. It must not contain domain-specific errors.

Repository and infrastructure packages should preserve technical context with `%w`:

```go
return fmt.Errorf("query users: %w", err)
```

These errors are for logs and tracing. They must not be returned directly to clients.

## 2. Common catalog

`internal/shared/apperror/common.go` contains errors whose meaning is identical in every module:

- invalid request, ID and pagination
- invalid or expired token
- permission denied
- generic resource not found
- generic internal failure

Handlers reference these values directly through `response.Error`.

## 3. Module catalogs

Every business module owns an `errors.go`. Examples:

- `identity/errors.go`: invalid credentials, disabled user, user not found
- `rbac/errors.go`: role/menu validation and lookup errors
- `sysconfig/errors.go`: readonly config, missing default, typed-value errors

An error belongs to a module when its wording or code expresses that module's business meaning. Do not duplicate common errors in each module.

## Internal causes

Module catalogs contain stable public errors. Attach an operation failure without mutating the catalog value:

```go
response.Error(ctx, apperror.WithCause(identity.ErrUserQueryInternal, err))
```

The client receives only the stable code/message/status. Logging middleware may inspect `errors.Unwrap` to record the cause after redaction.

## Rules

1. Do not use `errors.New` for API or domain errors.
2. Do not construct code/message pairs inside handlers.
3. Do not return `err.Error()` to clients.
4. Use existing common or module errors before adding a new one.
5. Give new domain modules an `errors.go` as part of their first API slice.
6. Preserve existing frontend application codes during migration.
7. Unknown errors are mapped to common code `600` without exposing the cause.