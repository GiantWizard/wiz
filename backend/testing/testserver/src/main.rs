use axum::{routing::get, Router, response::IntoResponse, http::StatusCode};
use chrono::Utc;
use std::error::Error;
use std::fs;
use std::env;
use std::net::SocketAddr;
use tokio::time::{sleep, Duration};
use tokio::process::Command;

/// Healthcheck endpoint that returns 200 OK.
async fn health_handler() -> impl IntoResponse {
    StatusCode::OK
}

/// Exports a file to Mega.nz using the Mega CLI command "mega-put".
/// It assumes that the environment variable MEGA_REMOTE_FOLDER is set.
async fn export_to_mega(file_path: &str) -> Result<(), Box<dyn Error + Send + Sync>> {
    let mega_remote_folder = env::var("MEGA_REMOTE_FOLDER")?;
    println!("Exporting {} to Mega.nz folder {}", file_path, mega_remote_folder);
    let status = Command::new("mega-put")
        .arg(file_path)
        .arg(&mega_remote_folder)
        .status()
        .await?;
    if !status.success() {
        return Err("mega-put command failed".into());
    }
    println!("Successfully exported {} to Mega.nz.", file_path);
    Ok(())
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    // Create a folder for exports.
    fs::create_dir_all("exports")?;

    // Start a minimal HTTP server for healthchecks.
    let port: u16 = env::var("PORT").unwrap_or_else(|_| "3000".to_string()).parse()?;
    let addr = SocketAddr::from(([0, 0, 0, 0], port));
    println!("Starting HTTP server on {}", addr);
    let app = Router::new()
        .route("/", get(health_handler))
        .route("/health", get(health_handler));
    tokio::spawn(async move {
        axum::Server::bind(&addr)
            .serve(app.into_make_service())
            .await
            .unwrap();
    });

    // Main loop: every 5 seconds, create a file with the current timestamp and export it.
    loop {
        let timestamp = Utc::now().format("%Y%m%d%H%M%S").to_string();
        let file_path = format!("exports/export_{}.txt", timestamp);
        fs::write(&file_path, format!("Timestamp: {}", timestamp))?;
        println!("Created export file: {}", file_path);

        if let Err(e) = export_to_mega(&file_path).await {
            eprintln!("Error exporting to Mega.nz: {}", e);
        }
        sleep(Duration::from_secs(5)).await;
    }
}
