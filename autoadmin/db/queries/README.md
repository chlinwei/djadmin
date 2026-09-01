# sqlc queries

Keep handwritten SQL grouped by owning domain, for example `user.sql`, `assets.sql` and `scheduler.sql`.

Every query must have a sqlc name annotation and accept `context.Context` through generated methods. Transaction boundaries belong in repository or service code, not HTTP handlers.