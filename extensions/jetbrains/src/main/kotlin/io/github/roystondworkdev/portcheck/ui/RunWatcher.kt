// Watches Run/Debug consoles for "port already in use" errors and offers to
// stop what holds the port and run the configuration again.
package io.github.roystondworkdev.portcheck.ui

import com.intellij.execution.ExecutionListener
import com.intellij.execution.process.BaseProcessHandler
import com.intellij.execution.process.ProcessEvent
import com.intellij.execution.process.ProcessHandler
import com.intellij.execution.process.ProcessListener
import com.intellij.execution.runners.ExecutionEnvironment
import com.intellij.execution.runners.ExecutionUtil
import com.intellij.notification.Notification
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationType
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.util.Key
import com.intellij.openapi.wm.ToolWindowManager
import io.github.roystondworkdev.portcheck.BusyPort
import io.github.roystondworkdev.portcheck.Fix
import io.github.roystondworkdev.portcheck.OutputScanner
import io.github.roystondworkdev.portcheck.PortError
import io.github.roystondworkdev.portcheck.busyPort
import java.awt.datatransfer.StringSelection

class RunWatcher : ExecutionListener {
    override fun processStarted(executorId: String, env: ExecutionEnvironment, handler: ProcessHandler) {
        if (!PortcheckSettings.get().state.watchConsoles) {
            return
        }
        val commandLine = (handler as? BaseProcessHandler<*>)?.commandLine
        val scanner = OutputScanner(commandLine)
        handler.addProcessListener(object : ProcessListener {
            override fun onTextAvailable(event: ProcessEvent, outputType: Key<*>) {
                scanner.push(event.text).forEach { offerFix(env, it) }
            }

            override fun processTerminated(event: ProcessEvent) {
                scanner.end().forEach { offerFix(env, it) }
            }
        })
    }

    private fun offerFix(env: ExecutionEnvironment, error: PortError) {
        val project = env.project
        Portcheck.background(project, { it.check(error.port) }) { data ->
            // Free again (the other process stopped meanwhile): nothing to offer.
            val busy = busyPort(data, error.port) ?: return@background
            Portcheck.notify(project, busy.summary, NotificationType.WARNING, false, *actions(env, busy).toTypedArray())
        }
    }

    private fun actions(env: ExecutionEnvironment, busy: BusyPort): List<NotificationAction> {
        val project = env.project
        val stops = busy.fixes.filterIsInstance<Fix.Stop>()
        val list = mutableListOf<NotificationAction>()
        if (stops.size == 1) {
            val s = stops.single()
            list += action("Stop ${s.name} and rerun") { n ->
                n.expire()
                stopProcess(project, s.pid) { ok -> if (ok) ExecutionUtil.restart(env) }
            }
            list += action("Stop ${s.name}") { n ->
                n.expire()
                stopProcess(project, s.pid)
            }
        }
        busy.fixes.filterIsInstance<Fix.Container>().firstOrNull()?.let { c ->
            list += action("Copy stop command") { CopyPasteManager.getInstance().setContents(StringSelection(c.stopHint)) }
        }
        if (Fix.Hidden in busy.fixes) {
            list += action("Copy command") { CopyPasteManager.getInstance().setContents(StringSelection("sudo portcheck kill ${busy.port}")) }
        }
        list += action("Show ports") { ToolWindowManager.getInstance(project).getToolWindow("Ports")?.show() }
        return list
    }

    private fun action(text: String, run: (Notification) -> Unit) = object : NotificationAction(text) {
        override fun actionPerformed(e: AnActionEvent, notification: Notification) = run(notification)
    }
}
