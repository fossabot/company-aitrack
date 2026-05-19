use clap::{Args, Parser, Subcommand};

#[derive(Parser)]
#[command(name = "aitrack", version, about = "Hardened AI coding edit telemetry CLI")]
pub struct Cli {
    #[command(subcommand)]
    pub command: Command,
}

#[derive(Subcommand)]
pub enum Command {
    /// Install hooks into AI coding tools
    Init(InitArgs),
    /// Remove hooks from AI coding tools
    Remove(RemoveArgs),
    /// Hook callback: reads stdin JSON and records an edit event
    Capture(CaptureArgs),
    /// Show recent local records
    Inspect(InspectArgs),
    /// Show record stats grouped by token
    Stats,
    /// Show status (token, pending count, hook installation)
    Status,
    /// Delete local records
    Clean(CleanArgs),
    /// Send a heartbeat to the server (forced, ignores throttle)
    Heartbeat,
    /// Hook callback: reads stdin JSON and records a user prompt
    PromptCapture(PromptCaptureArgs),
    /// Check for and install updates.
    Update {
        /// Only check for updates, do not install.
        #[arg(long)]
        check_only: bool,
        /// Force update even if already on latest version.
        #[arg(long)]
        force: bool,
    },
}

#[derive(Args)]
pub struct InitArgs {
    #[arg(long)]
    pub claude: bool,
    #[arg(long)]
    pub codex: bool,
    #[arg(long)]
    pub cursor: bool,
    #[arg(long)]
    pub api_url: Option<String>,
    /// Combined credential string: "<token>-<hmac_secret>"
    #[arg(long)]
    pub credential: Option<String>,
}

#[derive(Args)]
pub struct RemoveArgs {
    #[arg(long)]
    pub claude: bool,
    #[arg(long)]
    pub codex: bool,
    #[arg(long)]
    pub cursor: bool,
}

#[derive(Args)]
pub struct CaptureArgs {
    #[arg(short, long, default_value = "claude")]
    pub tool: String,
    #[arg(long)]
    pub api_url: Option<String>,
    /// Combined credential string: "<token>-<hmac_secret>"
    #[arg(long)]
    pub credential: Option<String>,
}

#[derive(Args)]
pub struct InspectArgs {
    #[arg(long, default_value_t = 20)]
    pub limit: i64,
    #[arg(long)]
    pub pending: bool,
    #[arg(long)]
    pub current_token: bool,
}

#[derive(Args)]
pub struct CleanArgs {
    #[arg(long)]
    pub all: bool,
    #[arg(long)]
    pub force: bool,
}

#[derive(Args)]
pub struct PromptCaptureArgs {
    #[arg(short, long, default_value = "claude")]
    pub tool: String,
}
