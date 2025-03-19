fn main() {
    // Create a simple JSON message
    let json_msg = r#"{"message": "Hello, MEGA!"}"#;
    
    // Credentials for the folder creation (replace with your actual username/password)
    let username = "bobofrogoo@gmail.com";
    let password = "numanuma321";
    
    println!("Rust: Sending JSON payload to exporter...");
    
    // Call the C++ exporter executable with the JSON, username, and password as arguments.
    let status = std::process::Command::new("./exporter")
        .arg(json_msg)
        .arg(username)
        .arg(password)
        .status()
        .expect("failed to execute exporter");
        
    if status.success() {
        println!("Rust: Exporter completed successfully.");
    } else {
        eprintln!("Rust: Exporter encountered an error.");
    }
}
