package io.github.roystondworkdev.portcheck.ui

import com.intellij.openapi.options.BoundConfigurable
import com.intellij.ui.dsl.builder.bindSelected
import com.intellij.ui.dsl.builder.bindText
import com.intellij.ui.dsl.builder.columns
import com.intellij.ui.dsl.builder.panel

/** Settings | Tools | portcheck. */
class SettingsConfigurable : BoundConfigurable("portcheck") {
    private val state get() = PortcheckSettings.get().state

    override fun createPanel() = panel {
        row("portcheck path:") {
            textField().bindText(state::path).columns(40)
                .comment("Leave empty to use the portcheck bundled with the plugin, or portcheck on PATH.")
        }
        row {
            checkBox("Offer to stop what's using a port when a Run/Debug console reports it busy")
                .bindSelected(state::watchConsoles)
        }
    }
}
