use actix_web::{get, App, HttpResponse, HttpServer, Responder};
use chrono::Local;
use std::env;
use std::fs::File;
use std::io::Write;
use std::process::Command;

/// GET handler for "/export"
#[get("/export")]
async fn export_file() -> impl Responder {
    // Retrieve MEGA_USER and MEGA_PASS from environment variables.
    let mega_user = match env::var("MEGA_USER") {
        Ok(val) => val,
        Err(_) => return HttpResponse::InternalServerError().body("MEGA_USER not set"),
    };
    let mega_pass = match env::var("MEGA_PASS") {
        Ok(val) => val,
        Err(_) => return HttpResponse::InternalServerError().body("MEGA_PASS not set"),
    };

    // Optionally, get the remote folder path from an environment variable (default to "/Exports" if not set).
    let mega_folder = env::var("MEGA_FOLDER").unwrap_or_else(|_| "/Exports".to_string());

    // Log out from any existing MEGA session.
    let _ = Command::new("mega-logout").output();

    // Log in using MEGA_USER and MEGA_PASS.
    let login_output = Command::new("mega-login")
        .arg(&mega_user)
        .arg(&mega_pass)
        .output();
    if let Err(e) = login_output {
        return HttpResponse::InternalServerError().body(format!("mega-login failed: {}", e));
    }
    if !login_output.unwrap().status.success() {
        return HttpResponse::InternalServerError().body("mega-login command failed");
    }

    // Generate a timestamp string and create a filename.
    let timestamp = Local::now().format("%Y%m%d%H%M%S").to_string();
    let filename = format!("export_{}.txt", timestamp);

    // Create and write content into the file.
    let mut file = match File::create(&filename) {
        Ok(f) => f,
        Err(e) => return HttpResponse::InternalServerError().body(format!("File creation error: {}", e)),
    };
    if let Err(e) = writeln!(file, "Exported at {}", timestamp) {
        return HttpResponse::InternalServerError().body(format!("File write error: {}", e));
    }

    // Construct the remote destination path (e.g. "/Exports/export_20250313061530.txt").
    let remote_path = format!("{}/{}", mega_folder, filename);

    // Use "mega-put" to upload the file to the remote folder.
    let upload_output = Command::new("mega-put")
        .arg(&filename)
        .arg(&remote_path)
        .output();

    // Check the result of the upload command.
    match upload_output {
        Ok(result) if result.status.success() => {
            // Optionally remove the local file after upload.
            let _ = std::fs::remove_file(&filename);
            HttpResponse::Ok().body(format!("File {} uploaded successfully to {}.", filename, remote_path))
        }
        Ok(result) => {
            let err_msg = String::from_utf8_lossy(&result.stderr);
            HttpResponse::InternalServerError().body(format!("Upload failed: {}", err_msg))
        }
        Err(e) => HttpResponse::InternalServerError().body(format!("Failed to run mega-put: {}", e)),
    }
}

/// The main function initializes and runs the Actix-web server.
#[actix_web::main]
async fn main() -> std::io::Result<()> {
    // Create and run the HTTP server on port 8080.
    HttpServer::new(|| App::new().service(export_file))
        .bind("0.0.0.0:8080")?
        .run()
        .await
}
