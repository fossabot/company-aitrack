use clap::Parser;
use aitrack::cli::Cli;

#[tokio::main]
async fn main() {
    // prompt-capture is invoked silently by hooks — skip banner to avoid
    // polluting hook stdout with decoration.
    let is_prompt_capture = std::env::args()
        .nth(1)
        .map(|a| a == "prompt-capture")
        .unwrap_or(false);
    if !is_prompt_capture {
        aitrack::print_banner();
    }

    let cli = Cli::parse();
    if let Err(e) = aitrack::run(cli).await {
        eprintln!("{e}");
        std::process::exit(1);
    }
}
