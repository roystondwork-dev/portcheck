# Architecture decision records

Significant design decisions are recorded as short ADRs: the context, the decision,
and its consequences. Each ADR is proposed in a pull request and left open for
feedback for at least 5 days; see
[GOVERNANCE.md](https://github.com/roystondwork-dev/portcheck/blob/main/GOVERNANCE.md#decision-making).

| ADR | Title | Status |
|---|---|---|
| [0001](0001-gopsutil-provider.md) | Use gopsutil behind a Provider interface for socket/process data | Accepted |
| [0002](0002-tcp-connect-only-scanning.md) | Remote scanning uses TCP connect only | Accepted |
| [0003](0003-native-macos-sockets.md) | Read macOS sockets through libproc instead of running lsof | Accepted |

To propose a new one, copy the latest ADR, number it sequentially, set **Status:
Proposed**, and open a pull request.
