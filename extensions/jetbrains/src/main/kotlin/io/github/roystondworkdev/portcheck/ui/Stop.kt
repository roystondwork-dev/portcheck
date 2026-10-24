// Stopping a process: always after confirming, always with portcheck's guards
// (it never stops system processes or the IDE itself).
package io.github.roystondworkdev.portcheck.ui

import com.intellij.notification.NotificationType
import com.intellij.openapi.project.Project
import com.intellij.openapi.ui.Messages
import io.github.roystondworkdev.portcheck.killSucceeded
import io.github.roystondworkdev.portcheck.killSummary

fun confirmStop(project: Project?, name: String, pid: Int): Boolean =
    Messages.showYesNoDialog(project, "Stop $name (pid $pid)?", "Stop Process", "Stop", "Cancel", Messages.getWarningIcon()) == Messages.YES

/** Stops pid; then(stopped) runs on the UI thread. */
fun stopProcess(project: Project?, pid: Int, then: (Boolean) -> Unit = {}) {
    Portcheck.background(project, { it.kill(pid) }) { data ->
        val ok = killSucceeded(data)
        Portcheck.notify(project, killSummary(data), if (ok) NotificationType.INFORMATION else NotificationType.WARNING)
        then(ok)
    }
}
