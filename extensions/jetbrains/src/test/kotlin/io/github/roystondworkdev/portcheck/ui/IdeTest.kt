package io.github.roystondworkdev.portcheck.ui

import com.intellij.execution.Executor
import com.intellij.execution.configurations.RunProfile
import com.intellij.execution.configurations.RunProfileState
import com.intellij.execution.executors.DefaultRunExecutor
import com.intellij.execution.process.NopProcessHandler
import com.intellij.execution.process.ProcessOutputTypes
import com.intellij.execution.runners.ExecutionEnvironment
import com.intellij.execution.runners.ExecutionEnvironmentBuilder
import com.intellij.execution.runners.ProgramRunner
import com.intellij.execution.configurations.RunnerSettings
import com.intellij.notification.Notification
import com.intellij.notification.Notifications
import com.intellij.openapi.util.SystemInfo
import com.intellij.testFramework.PlatformTestUtil
import com.intellij.testFramework.fixtures.BasePlatformTestCase
import com.intellij.ui.SearchTextField
import com.intellij.ui.table.JBTable
import com.intellij.util.ui.UIUtil
import java.io.File
import javax.swing.Icon

/**
 * Runs inside a headless IDE with a fake portcheck (a shell script printing
 * canned JSON), so the IDE wiring is tested without real processes.
 */
class IdeTest : BasePlatformTestCase() {
    private val notifications = mutableListOf<Notification>()

    // The fake portcheck is a shell script.
    override fun shouldRunTest() = !SystemInfo.isWindows && super.shouldRunTest()

    override fun setUp() {
        super.setUp()
        val check = """{"schemaVersion":1,"kind":"check","data":{"ports":[{"port":3000,"proto":"tcp","status":"in-use","holders":[
            {"proto":"tcp","localAddress":"::","localPort":3000,"state":"LISTEN","pid":4242,"process":{"pid":4242,"name":"node","ancestors":[{"pid":4200,"name":"npm"}]}}]}]}}"""
        val list = """{"schemaVersion":1,"kind":"list","data":{"sockets":[
            {"proto":"tcp","localAddress":"::","localPort":3000,"state":"LISTEN","pid":4242,"process":{"pid":4242,"name":"node","user":"alice"}},
            {"proto":"tcp","localAddress":"127.0.0.1","localPort":5432,"state":"LISTEN","pid":99,"process":{"pid":99,"name":"postgres","user":"alice"}}],"partial":false}}"""
        val script = File.createTempFile("portcheck", ".sh").apply {
            writeText("#!/bin/sh\ncase \"$1\" in\n  check) echo '${check.replace("\n", "")}'; exit 1;;\n  list) echo '${list.replace("\n", "")}';;\nesac\n")
            setExecutable(true)
            deleteOnExit()
        }
        PortcheckSettings.get().state.path = script.path
        // Notifications for a project are published on its message bus.
        project.messageBus.connect(testRootDisposable).subscribe(
            Notifications.TOPIC,
            object : Notifications {
                override fun notify(notification: Notification) {
                    notifications += notification
                }
            },
        )
    }

    override fun tearDown() {
        PortcheckSettings.get().state.path = ""
        super.tearDown()
    }

    private fun env(): ExecutionEnvironment {
        val profile = object : RunProfile {
            override fun getState(executor: Executor, environment: ExecutionEnvironment): RunProfileState? = null

            override fun getName() = "dev server"

            override fun getIcon(): Icon? = null
        }
        val runner = object : ProgramRunner<RunnerSettings> {
            override fun getRunnerId() = "portcheck-test"

            override fun canRun(executorId: String, profile: RunProfile) = true

            override fun execute(environment: ExecutionEnvironment) = Unit
        }
        return ExecutionEnvironmentBuilder(project, DefaultRunExecutor.getRunExecutorInstance()).runProfile(profile).runner(runner).build()
    }

    fun testRunConsoleErrorOffersToStopAndRerun() {
        val handler = NopProcessHandler()
        RunWatcher().processStarted(DefaultRunExecutor.EXECUTOR_ID, env(), handler)
        handler.startNotify()
        handler.notifyTextAvailable("\u001b[31mError: listen EADDRINUSE: address already in use :::3000\u001b[0m\n", ProcessOutputTypes.STDERR)
        PlatformTestUtil.waitWithEventsDispatching("no notification", { notifications.isNotEmpty() }, 15)

        val n = notifications.single()
        assertEquals("Port 3000 is in use by node (pid 4242), started by npm (4200).", n.content)
        assertEquals(listOf("Stop node and rerun", "Stop node", "Show ports"), n.actions.map { it.templateText })
    }

    fun testWatchingCanBeTurnedOff() {
        PortcheckSettings.get().state.watchConsoles = false
        try {
            val handler = NopProcessHandler()
            RunWatcher().processStarted(DefaultRunExecutor.EXECUTOR_ID, env(), handler)
            handler.startNotify()
            handler.notifyTextAvailable("Error: listen EADDRINUSE: address already in use :::3000\n", ProcessOutputTypes.STDERR)
            PlatformTestUtil.dispatchAllEventsInIdeEventQueue()
            Thread.sleep(500)
            PlatformTestUtil.dispatchAllEventsInIdeEventQueue()
            assertEmpty(notifications)
        } finally {
            PortcheckSettings.get().state.watchConsoles = true
        }
    }

    fun testPortsPanelListsAndFiltersPorts() {
        val panel = PortsPanel(project)
        panel.refresh()
        val table = UIUtil.findComponentOfType(panel, JBTable::class.java)!!
        PlatformTestUtil.waitWithEventsDispatching("no rows", { table.rowCount == 2 }, 15)
        assertEquals("node", table.getValueAt(0, 3))
        assertEquals("5432", table.getValueAt(1, 0))
        UIUtil.findComponentOfType(panel, SearchTextField::class.java)!!.text = "postgres"
        assertEquals(1, table.rowCount)
        assertEquals("postgres", table.getValueAt(0, 3))
    }
}
