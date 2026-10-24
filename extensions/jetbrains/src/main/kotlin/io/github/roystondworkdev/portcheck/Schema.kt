// Types for portcheck's JSON output, schema version 1.
// Source of truth: docs/json-schema.md in the portcheck repository.
package io.github.roystondworkdev.portcheck

const val SCHEMA_VERSION = 1

data class ProcessRef(val pid: Int = 0, val name: String = "")

data class ProcessInfo(
    val pid: Int = 0,
    val name: String = "",
    val user: String? = null,
    val cmdline: String? = null,
    val startTime: String? = null,
    val ancestors: List<ProcessRef>? = null,
)

data class ContainerInfo(
    val id: String? = null,
    val name: String? = null,
    val image: String? = null,
    val runtime: String = "",
    val stopHint: String = "",
)

data class Socket(
    val proto: String = "",
    val localAddress: String = "",
    val localPort: Int = 0,
    val state: String = "",
    val pid: Int? = null,
    val process: ProcessInfo? = null,
    val container: ContainerInfo? = null,
)

data class ListData(val sockets: List<Socket> = emptyList(), val partial: Boolean = false)

data class PortStatus(
    val port: Int = 0,
    val proto: String = "",
    /** "free", "in-use" or "denied" */
    val status: String = "",
    val holders: List<Socket> = emptyList(),
)

data class CheckData(val ports: List<PortStatus> = emptyList())

data class FreeData(val ports: List<Int> = emptyList())

data class KillResult(
    val pid: Int = 0,
    val name: String? = null,
    /** "terminated", "killed", "already-exited", "dry-run" or "failed" */
    val outcome: String = "",
    val error: String? = null,
)

data class KillData(val results: List<KillResult> = emptyList())

data class VersionData(val version: String = "")
