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