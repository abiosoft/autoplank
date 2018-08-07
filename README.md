# autoplank

Use plank on multi-monitor setup without creating multiple docks. Simply move the mouse to the bottom of any monitor and plank moves there.

## Usage

Start from the CLI.
```
autoplank
```

## Building/Installing

Requires Go 1.8 or newer.

```
go build -o autoplank && sudo mv autoplank /usr/local/bin
```

### [Optional] Create a service

You may want to create a service to start and keep running in background for convenience.

* create a systemd unit file `$HOME/.config/systemd/user/autoplank.service`

```systemd
[Unit]
Description=Autoplank Service

[Service]
ExecStart=/usr/local/bin/autoplank
Restart=always

[Install]
WantedBy=default.target
```
* enable and start
```
systemctl enable autoplank --user
systemctl start autoplank --user
```
