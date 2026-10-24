//! Zed extension: runs `portcheck mcp` as a context server, so Zed's agent
//! gets portcheck's tools (check_port, list_ports, find_free_port,
//! wait_for_port, stop_process; see docs/guide/mcp.md).

use zed_extension_api::{
    self as zed, serde_json, settings::ContextServerSettings, Command, ContextServerConfiguration,
    ContextServerId, Project, Result,
};

const INSTALL: &str = "Requires the free, open-source **portcheck** CLI:\n\n\
```sh\nbrew install roystondwork-dev/tap/portcheck\n```\n\n\
Other ways (Windows, Linux, npm, pip): https://roystondwork-dev.github.io/portcheck/install/\n\n\
If Zed can't find `portcheck`, set its full path (`which portcheck`) below.";

const SCHEMA: &str = r#"{
  "type": "object",
  "properties": {
    "path": { "type": "string", "description": "Path to the portcheck binary (default: portcheck on PATH)" }
  }
}"#;

const DEFAULTS: &str = r#"{
  "path": "portcheck"
}"#;

/// The portcheck binary: the "path" setting, else portcheck on PATH.
fn binary(settings: Option<&serde_json::Value>) -> String {
    settings
        .and_then(|s| s.get("path"))
        .and_then(|p| p.as_str())
        .map(str::trim)
        .filter(|p| !p.is_empty())
        .unwrap_or("portcheck")
        .to_string()
}

struct Portcheck;

impl zed::Extension for Portcheck {
    fn new() -> Self {
        Portcheck
    }

    fn context_server_command(
        &mut self,
        id: &ContextServerId,
        project: &Project,
    ) -> Result<Command> {
        let settings = ContextServerSettings::for_project(id.as_ref(), project)?;
        Ok(Command {
            command: binary(settings.settings.as_ref()),
            args: vec!["mcp".into()],
            env: vec![],
        })
    }

    fn context_server_configuration(
        &mut self,
        _id: &ContextServerId,
        _project: &Project,
    ) -> Result<Option<ContextServerConfiguration>> {
        Ok(Some(ContextServerConfiguration {
            installation_instructions: INSTALL.into(),
            settings_schema: SCHEMA.into(),
            default_settings: DEFAULTS.into(),
        }))
    }
}

zed::register_extension!(Portcheck);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn binary_from_settings() {
        assert_eq!(binary(None), "portcheck");
        assert_eq!(binary(Some(&serde_json::json!({}))), "portcheck");
        assert_eq!(
            binary(Some(&serde_json::json!({"path": "  "}))),
            "portcheck"
        );
        assert_eq!(binary(Some(&serde_json::json!({"path": 3}))), "portcheck");
        assert_eq!(
            binary(Some(
                &serde_json::json!({"path": " /opt/homebrew/bin/portcheck "})
            )),
            "/opt/homebrew/bin/portcheck"
        );
    }

    #[test]
    fn configuration_is_valid_json() {
        let schema: serde_json::Value = serde_json::from_str(SCHEMA).unwrap();
        assert_eq!(schema["properties"]["path"]["type"], "string");
        let defaults: serde_json::Value = serde_json::from_str(DEFAULTS).unwrap();
        assert_eq!(binary(Some(&defaults)), "portcheck");
    }
}
