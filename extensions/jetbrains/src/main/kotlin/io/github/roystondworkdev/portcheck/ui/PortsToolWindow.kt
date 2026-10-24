// The Ports tool window: every listening port, its process and user.
package io.github.roystondworkdev.portcheck.ui

import com.intellij.icons.AllIcons
import com.intellij.ide.BrowserUtil
import com.intellij.openapi.actionSystem.ActionManager
import com.intellij.openapi.actionSystem.ActionUpdateThread
import com.intellij.openapi.actionSystem.AnActionEvent
import com.intellij.openapi.actionSystem.DefaultActionGroup
import com.intellij.openapi.ide.CopyPasteManager
import com.intellij.openapi.project.DumbAwareAction
import com.intellij.openapi.project.Project
import com.intellij.openapi.wm.ToolWindow
import com.intellij.openapi.wm.ToolWindowFactory
import com.intellij.ui.DoubleClickListener
import com.intellij.ui.ScrollPaneFactory
import com.intellij.ui.SearchTextField
import com.intellij.ui.content.ContentFactory
import com.intellij.ui.table.JBTable
import com.intellij.util.ui.ColumnInfo
import com.intellij.util.ui.ListTableModel
import io.github.roystondworkdev.portcheck.PortRow
import io.github.roystondworkdev.portcheck.filterRows
import io.github.roystondworkdev.portcheck.portRows
import java.awt.BorderLayout
import java.awt.datatransfer.StringSelection
import java.awt.event.MouseEvent
import javax.swing.JPanel
import javax.swing.event.DocumentEvent

class PortsToolWindowFactory : ToolWindowFactory {
    override fun createToolWindowContent(project: Project, toolWindow: ToolWindow) {
        val panel = PortsPanel(project)
        toolWindow.contentManager.addContent(ContentFactory.getInstance().createContent(panel, null, false))
        panel.refresh()
    }
}

private fun column(name: String, value: (PortRow) -> String) = object : ColumnInfo<PortRow, String>(name) {
    override fun valueOf(item: PortRow) = value(item)
}

class PortsPanel(private val project: Project) : JPanel(BorderLayout()) {
    private val model = ListTableModel<PortRow>(
        column("Port") { it.port.toString() },
        column("Proto") { it.proto },
        column("Address") { it.address },
        column("Process") { it.process },
        column("PID") { it.pid?.toString().orEmpty() },
        column("User") { it.user },
    )
    private val table = JBTable(model).apply {
        emptyText.text = "Loading…"
        setShowGrid(false)
    }
    private val filter = SearchTextField(false)
    private var rows = emptyList<PortRow>()

    init {
        val actions = DefaultActionGroup(
            action("Refresh", AllIcons.Actions.Refresh) { refresh() },
            action("Stop Process", AllIcons.Actions.Suspend, { selected()?.pid != null }) { selected()?.let { stopRow(it) } },
            action("Open in Browser", AllIcons.General.Web, { selected() != null }) { selected()?.let { BrowserUtil.browse(it.url) } },
            action("Copy PID", AllIcons.Actions.Copy, { selected()?.pid != null }) {
                selected()?.pid?.let { CopyPasteManager.getInstance().setContents(StringSelection(it.toString())) }
            },
        )
        val toolbar = ActionManager.getInstance().createActionToolbar("PortcheckPorts", actions, true).apply { targetComponent = table }
        val top = JPanel(BorderLayout()).apply {
            add(toolbar.component, BorderLayout.WEST)
            add(filter, BorderLayout.CENTER)
        }
        filter.textEditor.toolTipText = "Filter by port, process or user"
        filter.addDocumentListener(object : com.intellij.ui.DocumentAdapter() {
            override fun textChanged(e: DocumentEvent) = render()
        })
        object : DoubleClickListener() {
            override fun onDoubleClick(event: MouseEvent): Boolean {
                selected()?.let { BrowserUtil.browse(it.url) }
                return true
            }
        }.installOn(table)
        add(top, BorderLayout.NORTH)
        add(ScrollPaneFactory.createScrollPane(table), BorderLayout.CENTER)
    }

    private fun selected(): PortRow? = table.selectedRow.takeIf { it >= 0 }?.let { model.getItem(table.convertRowIndexToModel(it)) }

    private fun render() {
        model.items = filterRows(rows, filter.text)
        table.emptyText.text = if (rows.isEmpty()) "Nothing is listening." else "No ports match."
    }

    fun refresh() {
        Portcheck.background(project, { it.list() }) { data ->
            rows = portRows(data)
            render()
        }
    }

    private fun stopRow(row: PortRow) {
        val pid = row.pid ?: return
        if (confirmStop(project, row.process, pid)) {
            stopProcess(project, pid) { refresh() }
        }
    }

    private fun action(text: String, icon: javax.swing.Icon, enabled: () -> Boolean = { true }, run: () -> Unit) =
        object : DumbAwareAction(text, null, icon) {
            override fun actionPerformed(e: AnActionEvent) = run()

            override fun update(e: AnActionEvent) {
                e.presentation.isEnabled = enabled()
            }

            override fun getActionUpdateThread() = ActionUpdateThread.EDT
        }
}
