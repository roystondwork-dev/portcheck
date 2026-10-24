// Tools | portcheck actions (also in Find Action / Search Everywhere).
package io.github.roystondworkdev.portcheck.ui

import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.DumbAwareAction
import com.intellij.openapi.ui.InputValidator
import com.intellij.openapi.ui.Messages
import io.github.roystondworkdev.portcheck.Fix
import io.github.roystondworkdev.portcheck.busyPort
import java.awt.datatransfer.StringSelection

private object PortValidator : InputValidator {
    override fun checkInput(input: String?) = input?.trim()?.toIntOrNull() in 1..65535

    override fun canClose(input: String?) = checkInput(input)
}

private fun askPort(e: AnActionEvent, title: String): Int? =
    Messages.showInputDialog(e.project, "Port:", title, null, "3000", PortValidator)?.trim()?.toIntOrNull()

class CheckPortAction : DumbAwareAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val port = askPort(e, "Check a Port") ?: return
        Portcheck.background(e.project, { it.check(port) }) { data ->
            Portcheck.notify(e.project, busyPort(data, port)?.summary ?: "Port $port is free.")
        }
    }
}

class FindFreePortAction : DumbAwareAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val range = Messages.showInputDialog(e.project, "Range:", "Find a Free Port", null, "3000-3999", null)?.trim() ?: return
        Portcheck.background(e.project, { it.free(range) }) { data ->
            val port = data.ports.firstOrNull()
            if (port == null) {
                Portcheck.notify(e.project, "No free port in $range.", NotificationType.WARNING)
            } else {
                CopyPasteManager.getInstance().setContents(StringSelection(port.toString()))
                Portcheck.notify(e.project, "Port $port is free (copied).")
            }
        }
    }
}

class StopPortAction : DumbAwareAction() {
    override fun actionPerformed(e: AnActionEvent) {
        val port = askPort(e, "Stop What's Using a Port") ?: return
        val project = e.project
        Portcheck.background(project, { it.check(port) }) { data ->
            val busy = busyPort(data, port)
            val stop = busy?.fixes?.filterIsInstance<Fix.Stop>()?.firstOrNull()
            when {
                busy == null -> Portcheck.notify(project, "Port $port is free.")
                stop == null -> Portcheck.notify(project, busy.summary, NotificationType.WARNING)
                confirmStop(project, stop.name, stop.pid) -> stopProcess(project, stop.pid)
            }
        }
    }
}
