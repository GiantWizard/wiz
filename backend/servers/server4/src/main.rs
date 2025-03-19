use std::env;
use std::error::Error;
use std::time::Duration;
use chrono::Utc;
use tokio::time;
use mega::{Client, ClientBuilder, Node, NodeKind, LastModified};
use std::fs;
use std::io::Write;
use serde_json::json;

#[tokio::main]
async fn main() -> Result<(), Box<dyn Error>> {
    println!("Starting Mega.nz exporter using mega-rs");
    println!("Current Date and Time (UTC - YYYY-MM-DD HH:MM:SS formatted): {}", 
              Utc::now().format("%Y-%m-%d %H:%M:%S").to_string());
    println!("Current User's Login: GiantWizard");
    
    // Create a local directory for exports (for backup)
    let export_dir = "exports";
    fs::create_dir_all(export_dir)?;
    
    println!("Created local exports directory at: {}", export_dir);
    
    // Try to connect to Mega.nz
    let client = create_mega_client().await?;
    println!("Connected to Mega.nz successfully!");
    
    // Get user info
    let user_info = client.get_user_info().await?;
    println!("MEGA Account: {}", user_info.email);
    
    // Get storage quototas
    let storage_quotas = client.get_storage_quotas().await?;
    println!("Storage used: {} bytes out of {} bytes", 
              storage_quotas.used_storage_space, 
              storage_quotas.total_storage_space);
    
    // Find or create an exports folder
    let nodes = client.fetch_nodes().await?;
    let root_node = nodes.get_root_node();
    
    println!("Found root node: {}", root_node.name());
    
    // Look for exports folder
    let mut exports_folder = None;
    for node in nodes.get_children_of_node(&root_node) {
        if node.name() == "exports" && node.kind() == NodeKind::Folder {
            println!("Found existing exports folder");
            exports_folder = Some(node);
            break;
        }
    }
    
    // Create exports folder if it doesn't exist
    let exports_folder = match exports_folder {
        Some(folder) => folder,
        None => {
            println!("Creating exports folder...");
            client.create_folder("exports", &root_node).await?
        }
    };
    
    println!("Using exports folder: {}", exports_folder.name());
    
    // Start periodic file export
    println!("Starting periodic file export (every 10 seconds)...");
    
    let mut interval = time::interval(Duration::from_secs(10));
    let mut file_counter = 0;
    
    // File export loop
    loop {
        interval.tick().await;
        file_counter += 1;
        
        println!("\n--- Starting new export {} ---", file_counter);
        
        // Create timestamp
        let timestamp = Utc::now().format("%Y-%m-%d %H:%M:%S").to_string();
        println!("Creating JSON file with timestamp: {}", timestamp);
        
        // Create JSON content
        let data = json!({
            "timestamp": timestamp,
            "user": "GiantWizard",
            "counter": file_counter
        });
        
        let json_content = serde_json::to_string_pretty(&data)?;
        
        // Filename with timestamp
        let filename = format!("data_{}.json", Utc::now().format("%Y%m%d%H%M%S").to_string());
        let file_path = format!("{}/{}", export_dir, filename);
        
        // Write to local file (backup)
        let mut file = fs::File::create(&file_path)?;
        file.write_all(json_content.as_bytes())?;
        
        println!("Local backup file created: {}", file_path);
        
        // Upload to Mega
        println!("Uploading to Mega.nz...");
        
        // Create a temporary file for upload
        let temp_file_name = format!("./temp_{}.json", Utc::now().format("%Y%m%d%H%M%S").to_string());
        {
            let mut temp_file = fs::File::create(&temp_file_name)?;
            temp_file.write_all(json_content.as_bytes())?;
        }
        
        // Upload the file
        match client.upload_file(&temp_file_name, &exports_folder, 
                                &filename, LastModified::Now).await {
            Ok(file_node) => {
                println!("Successfully uploaded file to Mega: {}", file_node.name());
                println!("File contents: timestamp={}, counter={}", timestamp, file_counter);
                
                // Try to create a public link if needed
                match client.create_public_link(&file_node).await {
                    Ok(link) => println!("Public link: {}", link),
                    Err(e) => println!("Note: Couldn't create public link (non-critical): {}", e),
                }
            },
            Err(e) => {
                println!("Failed to upload file to Mega: {}", e);
                println!("File is still available locally at: {}", file_path);
            }
        }
        
        // Clean up temp file
        match fs::remove_file(&temp_file_name) {
            Ok(_) => println!("Temporary file removed"),
            Err(e) => println!("Note: Couldn't remove temporary file (non-critical): {}", e),
        }
        
        println!("Waiting for next export cycle...");
    }
}

async fn create_mega_client() -> Result<Client, Box<dyn Error>> {
    println!("Connecting to Mega.nz...");
    
    // Get credentials from environment variables
    let email = env::var("MEGA_EMAIL").expect("MEGA_EMAIL not set");
    let password = env::var("MEGA_PASSWORD").expect("MEGA_PASSWORD not set");
    
    // Try to create client with exponential backoff retry strategy
    let mut retry_count = 0;
    let max_retries = 5;
    
    while retry_count < max_retries {
        println!("Login attempt {} of {}", retry_count + 1, max_retries);
        
        // Build client with timeout and retry options
        let client_result = ClientBuilder::new()
            .with_timeout(Duration::from_secs(30))
            .build()
            .and_then(|client| {
                // Try to login
                match client.login(&email, &password) {
                    Ok(_) => Ok(client),
                    Err(e) => {
                        println!("Login error: {:?}", e);
                        Err(e)
                    }
                }
            });
        
        match client_result {
            Ok(client) => {
                println!("Login successful!");
                return Ok(client);
            },
            Err(e) => {
                // For 402 Payment Required errors, try anonymous login
                if format!("{:?}", e).contains("402") {
                    println!("Payment required error detected, trying anonymous login...");
                    
                    // Try anonymous login
                    let anon_client = ClientBuilder::new()
                        .with_timeout(Duration::from_secs(30))
                        .build()?;
                    
                    println!("Created anonymous session. Note: functionality will be limited.");
                    return Ok(anon_client);
                }
                
                println!("Login attempt failed: {:?}", e);
                retry_count += 1;
                
                if retry_count < max_retries {
                    let wait_time = 2_u64.pow(retry_count as u32);
                    println!("Retrying in {} seconds...", wait_time);
                    time::sleep(Duration::from_secs(wait_time)).await;
                }
            }
        }
    }
    
    // If all login attempts fail, try one last approach: using a fresh anonymous client
    println!("All login attempts failed. Creating anonymous client as fallback...");
    let client = ClientBuilder::new()
        .with_timeout(Duration::from_secs(30))
        .build()?;
    
    Ok(client)
}