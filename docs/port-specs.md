# Port specs

Wherever portcheck takes ports (`check`, `free`, `kill`, `list --port`, `scan -p`), you can write:

| Form | Example | Meaning |
|---|---|---|
| single port | `80` | port 80 |
| list | `22,80,443` | each port |
| range | `8000-8100` | inclusive range |
| preset | `common` | a named set (below) |
| mixed | `common,9000-9010` | any combination |

Whitespace around elements is ignored. Overlapping and adjacent entries are merged,
so every port is used once. Commands that accept several arguments, such as
`portcheck check 80 443`, join them with commas.

Ports must be between 1 and 65535; `0`, reversed ranges like `100-10`, and empty
elements like `80,,443` are rejected with a clear error.

## Presets

Preset names are case-insensitive.

| Preset | Ports |
|---|---|
| `common` | 45 ports developers commonly use: ssh, http(s), databases (5432, 3306, 27017, 6379, …), message brokers, dev servers (3000, 4200, 5173, 8000, 8080, …) |
| `well-known` | 1-1023 |
| `registered` | 1024-49151 |
| `dynamic` | 49152-65535 (the IANA ephemeral range) |
| `all` | 1-65535 |

!!! note
    portcheck does not ship "top N ports" lists derived from other scanners, because
    they come with incompatible licenses. `common` is maintained by hand in
    [`pkg/portspec/presets.go`](https://github.com/roystondwork-dev/portcheck/blob/main/pkg/portspec/presets.go).
    Pull requests are welcome.
