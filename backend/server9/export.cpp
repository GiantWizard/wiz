#include <iostream>
#include <fstream>
#include <cstdlib>
// #include <csignal> // Not used
// #include <unistd.h> // Not used for core logic here
#include <sstream>
#include <stdexcept>
#include <vector> // For splitting string if needed (not strictly now)
#include <string> // For std::string

// It's good practice to use std:: consistently or declare `using namespace std;`
// For clarity in examples, often explicit std:: is used.
using namespace std;

// Executes a shell command and captures its output.
// Made it return the output string for potential future use.
string safeSystem(const string& cmd, bool checkError = true) {
    cout << "Executing: " << cmd << "\n";
    string fullCmd = cmd + " 2>&1"; // Capture both stdout and stderr
    FILE* pipe = popen(fullCmd.c_str(), "r");
    if (!pipe) {
        throw runtime_error("Failed to execute popen for command: " + cmd);
    }
    stringstream output_ss;
    char buffer[256];
    while (fgets(buffer, sizeof(buffer), pipe) != nullptr) {
        output_ss << buffer;
    }
    int status = pclose(pipe); // pclose returns -1 on error, or the exit status

    string output_str = output_ss.str();
    cout << output_str; // Print captured output

    if (checkError) {
        // WIFEXITED checks if the child terminated normally
        // WEXITSTATUS gets the exit code if WIFEXITED is true
        // A non-zero exit status typically indicates an error.
        if (status == -1 || (WIFEXITED(status) && WEXITSTATUS(status) != 0)) {
             throw runtime_error("Command failed with status " + to_string(WEXITSTATUS(status)) + ". Output: " + output_str);
        }
    }
    return output_str;
}

void validateLogin(const string& remote_dir) {
    safeSystem("mega-whoami", false); // Check current session, don't fail if not logged in
    safeSystem("mega-logout", false);  // Attempt logout, don't fail if not logged in
    
    const char* username = getenv("MEGA_EMAIL");
    const char* password = getenv("MEGA_PWD");
    if (!username || !password) {
        throw runtime_error("Missing MEGA_EMAIL or MEGA_PWD environment variables for login");
    }
    
    string loginCmd = "mega-login "; // Removed -v for potentially less verbose output
    loginCmd += username;
    loginCmd += " ";
    loginCmd += password;
    safeSystem(loginCmd);

    string mkdirCmd = "mega-mkdir -p \""; // Ensure quoting for paths with spaces
    mkdirCmd += remote_dir;
    mkdirCmd += "\"";
    
    // For mega-mkdir, a non-zero exit code if folder exists is common.
    // We need to check the output string.
    try {
        string mkdir_output = safeSystem(mkdirCmd, false); // Don't throw on non-zero yet
        // A more robust check would be to parse `mega-ls` output if `mega-mkdir` doesn't have a clear success/exists message.
        // For now, we assume if it didn't throw an *execution* error, and didn't complain about critical failure, it's okay.
        // The original code's string search is okay but fragile.
        if (mkdir_output.find("Problem creating remote node") != string::npos &&
            mkdir_output.find("Object (typically, folder) already exists") == string::npos) {
            // A real problem occurred other than "already exists"
            throw runtime_error("mega-mkdir failed: " + mkdir_output);
        } else if (mkdir_output.find("Folder already exists") != string::npos) {
             cout << "Folder " << remote_dir << " already exists, proceeding...\n";
        } else {
             cout << "mega-mkdir command for " << remote_dir << " completed.\n";
        }
    } catch (const runtime_error &e) {
        // This catches errors from safeSystem popen/pclose itself or if an unexpected error message was found.
        // If the error message indicates the folder already exists, we can ignore it.
        string errMsg = e.what();
        if (errMsg.find("Folder already exists") != string::npos || 
            errMsg.find("Object (typically, folder) already exists") != string::npos) {
            cout << "Folder " << remote_dir << " already exists (caught exception), proceeding...\n";
        } else {
            throw; // Re-throw other errors
        }
    }
}

int main(int argc, char* argv[]) {
    if (argc < 3) {
        cerr << "Usage: " << argv[0] << " <metrics_filename> <remote_dir>\n";
        return EXIT_FAILURE;
    }
    string filename = argv[1];
    string remote_dir = argv[2];

    try {
        validateLogin(remote_dir);

        string putCmd = "mega-put \"";
        putCmd += filename;
        putCmd += "\" \"";
        putCmd += remote_dir;
        putCmd += "\"";
        safeSystem(putCmd);
        cout << "Uploaded metrics file: " << filename << " to " << remote_dir << "\n";
        
        if (remove(filename.c_str()) != 0) {
            perror(("Warning: Could not delete local file " + filename).c_str());
        } else {
            cout << "Deleted local metrics file: " << filename << "\n";
        }
        safeSystem("mega-logout", false); // Logout, don't fail if already logged out
    } catch (const exception& e) {
        cerr << "Fatal error in export_engine: " << e.what() << "\n";
        safeSystem("mega-logout", false); // Attempt logout on error too
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}