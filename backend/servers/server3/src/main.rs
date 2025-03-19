use tokio::process::Command;
use anyhow::Result;
use std::env;

#[tokio::main]
async fn main() -> Result<()> {
    // Read credentials and remote folder from environment variables.
    let email = env::var("MEGA_USER")?;
    let password = env::var("MEGA_PASS")?;
    // Trim any leading or trailing slashes.
    let folder = env::var("MEGA_REMOTE_FOLDER")?.trim_matches('/').to_string();

    println!("Attempting to log out from Mega.nz (if already logged in)...");
    // Attempt to log out (ignore errors if not logged in).
    if let Err(e) = Command::new("mega-logout").status().await {
        println!("Error running mega-logout: {}. Proceeding...", e);
    }

    println!("Attempting to log in to Mega.nz with email: {}", email);
    // Run the mega-login command.
    let login_status = Command::new("mega-login")
        .arg(&email)
        .arg(&password)
        .status()
        .await?;
    if !login_status.success() {
        eprintln!("Error: mega-login command failed.");
        return Err(anyhow::anyhow!("mega-login command failed"));
    }
    println!("Logged in successfully.");

    println!("Attempting to create folder: {}", folder);
    // Attempt to create the folder.
    let output = Command::new("mega-mkdir")
        .arg(&folder)
        .output()
        .await?;
    if output.status.success() {
        println!("Folder '{}' created successfully.", folder);
    } else {
        eprintln!("Failed to create folder '{}'.", folder);
        let stderr = String::from_utf8_lossy(&output.stderr);
        eprintln!("Error output: {}", stderr);
    }

    Ok(())
}
