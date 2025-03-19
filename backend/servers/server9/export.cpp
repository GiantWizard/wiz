#include <iostream>
#include <fstream>
#include <cstdlib>
#include <csignal>
#include <unistd.h>
#include <sstream>
#include <stdexcept>
#include <cstring>

using namespace std;

// Executes a shell command and captures its output.
void safeSystem(const string& cmd, bool checkError = true) {
    cout << "Executing: " << cmd << "\n";
    string fullCmd = cmd + " 2>&1";
    FILE* pipe = popen(fullCmd.c_str(), "r");
    if (!pipe) {
        throw runtime_error("Failed to execute command");
    }
    stringstream output;
    char buffer[256];
    while (fgets(buffer, sizeof(buffer), pipe)) {
        output << buffer;
    }
    int status = pclose(pipe);
    if (checkError && status != 0) {
        throw runtime_error("Command failed: " + output.str());
    }
    cout << output.str();
}

// Logs in to MEGA using Railway environment variables and creates the target folder.
void validateLogin(const string& remote_dir) {
    // Clear any existing sessions.
    safeSystem("mega-whoami", false);
    safeSystem("mega-logout", false);
    
    const char* username = getenv("MEGA_USERNAME");
    const char* password = getenv("MEGA_PASSWORD");
    if (!username || !password) {
        throw runtime_error("Missing environment variables for login");
    }
    
    string loginCmd = "mega-login -v " + string(username) + " " + string(password);
    safeSystem(loginCmd);

    // Create target folder (ignore error if it already exists).
    try {
        safeSystem("mega-mkdir -p \"" + remote_dir + "\"");
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        if (errMsg.find("Folder already exists") != string::npos) {
            cout << "Folder already exists, proceeding...\n";
        } else {
            throw;
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
        // Validate login and ensure remote directory exists.
        validateLogin(remote_dir);

        // Export the metrics file to the remote directory.
        safeSystem("mega-put \"" + filename + "\" \"" + remote_dir + "\"");
        cout << "Uploaded metrics file: " << filename << " to " << remote_dir << "\n";
        
        // Delete the local file after a successful upload.
        if (remove(filename.c_str()) != 0) {
            cerr << "Warning: Could not delete file: " << filename << "\n";
        } else {
            cout << "Deleted local metrics file: " << filename << "\n";
        }
        safeSystem("mega-logout");
    } catch (const exception& e) {
        safeSystem("mega-logout", false);
        cerr << "Fatal error: " << e.what() << "\n";
        return EXIT_FAILURE;
    }
    return EXIT_SUCCESS;
}
