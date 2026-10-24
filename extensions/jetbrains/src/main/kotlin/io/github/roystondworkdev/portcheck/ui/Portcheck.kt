// The IDE's access to portcheck: which binary, notifications, and running
// calls off the UI thread.
package io.github.roystondworkdev.portcheck.ui

import com.intellij.ide.plugins.PluginManager
import com.intellij.notification.Notification
import com.intellij.notification.NotificationAction
import com.intellij.notification.NotificationGroupManager
import com.intellij.notification.NotificationType
import com.intellij.openapi.application.ApplicationManager
import com.intellij.openapi.components.PersistentStateComponent
import com.intellij.openapi.components.Service
import com.intellij.openapi.components.State
import com.intellij.openapi.components.Storage
import com.intellij.openapi.components.service
import com.intellij.openapi.project.Project
import io.github.roystondworkdev.portcheck.PortcheckClient
import io.github.roystondworkdev.portcheck.PortcheckException
import io.github.roystondworkdev.portcheck.ProcessExec
import io.github.roystondworkdev.portcheck.resolveBinary

@Service(Service.Level.APP)
@State(name = "Portcheck", storages = [Storage("portcheck.xml")])
class PortcheckSettings : PersistentStateComponent<PortcheckSettings.State> {
    class State {
        /** Path to portcheck; empty: the bundled one, else portcheck on PATH. */
        var path: String = ""

        /** Offer fixes when a Run/Debug console reports a busy port. */
        var watchConsoles: Boolean = true
    }

    private var state = State()

    override fun getState() = state

    override fun loadState(state: State) {
        this.state = state
    }

    companion object {
        fun get(): PortcheckSettings = service()
    }
}

object Portcheck {
    fun client(): PortcheckClient {
        val dir = PluginManager.getPluginByClass(Portcheck::class.java)?.pluginPath?.toFile()
        val binary = resolveBinary(PortcheckSettings.get().state.path, dir, System.getProperty("os.name"), System.getProperty("os.arch")) { it.isFile }
        return PortcheckClient(binary, ProcessExec)
    }

    /** Runs work off the UI thread, then done(result) on it. Errors become a notification. */
    fun <T> background(project: Project?, work: (PortcheckClient) -> T, done: (T) -> Unit) {
        val app = ApplicationManager.getApplication()
        app.executeOnPooledThread {
            val result = try {
                Result.success(work(client()))
            } catch (e: PortcheckException) {
                Result.failure(e)
            }
            app.invokeLater {
                if (project?.isDisposed == true) {
                    return@invokeLater
                }
                result.fold(done) { notify(project, it.message ?: "portcheck failed", NotificationType.ERROR, installHint = true) }
            }
        }
    }

    fun notify(project: Project?, content: String, type: NotificationType = NotificationType.INFORMATION, installHint: Boolean = false, vararg actions: NotificationAction): Notification {
        val n = NotificationGroupManager.getInstance().getNotificationGroup("portcheck").createNotification("portcheck", content, type)
        actions.forEach { n.addAction(it) }
        if (installHint && content.contains("couldn't be started")) {
            n.setContent("$content<br>Install portcheck, or set its path in Settings | Tools | portcheck.")
        }
        n.notify(project)
        return n
    }
}
