# autoadmin systemd deployment

Build the static Linux binary through the repository Makefile:

```bash
make build
```

Create a real environment file outside Git from `config.example.env`. At minimum, the API service requires non-empty `MYSQL_DSN`, `JWT_SECRET`, and `RABBITMQ_URL` values.

Install and enable the unit without starting it:

```bash
sudo ./deploy/install.sh --binary ./bin/autoadmin --config /path/to/config.env
```

Install and start immediately:

```bash
sudo ./deploy/install.sh --binary ./bin/autoadmin --config /path/to/config.env --start
```

Common operations:

```bash
sudo systemctl start autoadmin
sudo systemctl restart autoadmin
sudo systemctl stop autoadmin
sudo systemctl status autoadmin --no-pager
sudo journalctl -u autoadmin -f
```

The installed paths are:

- Binary: `/usr/local/bin/autoadmin`
- Environment: `/etc/autoadmin/config.env`
- Unit: `/etc/systemd/system/autoadmin.service`

## API instance and restart boundary

Run **one `autoadmin api` instance per deployment**. Stop the old instance before starting its replacement; do not use overlapping rolling updates or run a second API replica behind the load balancer. Agent gRPC streams are local to the API process, and startup reconciliation assumes the previous executor has exited. The `scheduler` and `worker` roles do not take over Agent connections.

On `SIGTERM` the API stops taking new baseline and inspection work and allows active Agent requests to finish within `SHUTDOWN_TIMEOUT` (default `10s`). Agent gRPC stays available during this drain. Make systemd's `TimeoutStopSec` longer than `SHUTDOWN_TIMEOUT` when overriding either value. On restart, unclaimed targets can resume from their snapshots; targets whose execution could not be confirmed are not automatically dispatched again.

To validate in a test deployment, start an inspection or baseline scan with more targets than its concurrency limit, then run `sudo systemctl restart autoadmin`. Check that the old PID exits before the replacement starts, that queued targets resume after Agent reconnection, and that unconfirmed targets are reported as interrupted rather than silently retried.