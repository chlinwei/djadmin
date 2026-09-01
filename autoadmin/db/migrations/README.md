# Migration policy

The existing Django-managed MySQL schema is the initial baseline. Do not run a second set of create-table migrations against production.

Adoption sequence:

1. Record the latest applied Django migration set.
2. Export and checksum the resulting schema.
3. Create a no-op Go migration baseline at that version.
4. Run all later schema changes through `golang-migrate` after the corresponding Go domain owns production traffic.
5. Keep rollback SQL for reversible changes; document irreversible data migrations explicitly.