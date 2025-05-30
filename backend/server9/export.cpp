#include <iostream>
#include <fstream>
#include <cstdlib>    // For getenv
#include <cstdio>     // For popen, pclose, fgets, remove, perror
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>
#include <sys/wait.h> // For WIFEXITED, WEXITSTATUS, WTERMSIG (on POSIX systems)
// #include <unistd.h> // For sleep, if you re-add it

using namespace std;

// Executes a shell command and captures its output.
// Throws runtime_error on failure if checkError is true,
// unless the exit_code is in allowedExitCodes.
// Returns the captured standard output of the command.
string safeSystem(const string& cmd, bool checkError = true, const vector<int>& allowedExitCodes = {}) {
    string effective_cmd = cmd;
    if (cmd.rfind("mega-", 0) == 0) { // Check if command starts with "mega-"
        effective_cmd = "env HOME=/home/appuser " + cmd;
    }
    cout << "Export Engine Executing: " << effective_cmd << endl;

    FILE* pipe = popen(effective_cmd.c_str(), "r");
    if (!pipe) {
        throw runtime_error("Failed to execute popen for command: " + effective_cmd);
    }

    stringstream output_stream;
    char buffer[256];
    string line;

    while (fgets(buffer, sizeof(buffer), pipe) != NULL) {
        line = buffer;
        // Trim trailing newline characters for cleaner multi-line logging
        while (!line.empty() && (line.back() == '\n' || line.back() == '\r')) {
            line.pop_back();
        }
        output_stream << line << endl; 
    }
    string cmd_output_str = output_stream.str();

    int status = pclose(pipe);
    int exit_code = -1; 

    if (WIFEXITED(status)) {
        exit_code = WEXITSTATUS(status);
        cout << "Export Engine: Command finished. Exit Code: " << exit_code << endl;
    } else if (WIFSIGNALED(status)) {
        cout << "Export Engine: Command terminated by signal: " << WTERMSIG(status) << endl;
    } else {
        cout << "Export Engine: Command did not exit normally. Raw Status: " << status << endl;
    }

    if (!cmd_output_str.empty()) {
        cout << "Export Engine Command Output:\n" << cmd_output_str << endl;
    } else {
        cout << "Export Engine: Command produced no direct output to stdout via pipe." << endl;
    }

    bool isAllowedExitCode = false;
    if (exit_code != -1) { // Only check allowedExitCodes if we got a normal exit code
        for (int allowed_code : allowedExitCodes) {
            if (exit_code == allowed_code) {
                isAllowedExitCode = true;
                break;
            }
        }
    }

    if (checkError && exit_code != 0 && !isAllowedExitCode) {
        string error_msg = "Export Engine: Command [" + effective_cmd + "] failed ";
        if (exit_code != -1) { 
            error_msg += "with exit code " + to_string(exit_code);
        } else { 
            error_msg += "(abnormal termination, status: " + to_string(status) + ")";
        }
        if(!cmd_output_str.empty()) { 
             error_msg += ". Output was: " + cmd_output_str; // Changed to "Output was:" for clarity
        }
        throw runtime_error(error_msg);
    }
    return cmd_output_str; // Return the command output
}


void validateLoginAndPrepareRemoteDir(const string& remote_dir) {
    cout << "Export Engine: Preparing MEGA session..." << endl;
    
    // Try to ensure a clean state. Call with checkError=false.
    safeSystem("mega-logout", false);      
    safeSystem("mega-ipc killserver", false); 
    safeSystem("mega-ipc wipeme", false);   
    // sleep(1); // Optional: short delay for server to fully terminate

    const char* email_env = getenv("MEGA_EMAIL");
    const char* password_env = getenv("MEGA_PWD");
    string email = email_env ? string(email_env) : "";
    string password = password_env ? string(password_env) : "";

    if (email.empty() || password.empty()) {
        throw runtime_error("Missing environment variables for login (MEGA_EMAIL or MEGA_PWD not found/empty)");
    }
    cout << "Export Engine: Attempting MEGA login for user: " << email << endl;
    string loginCmd = "mega-login \"" + email + "\" \"" + password + "\"";
    string loginOutput;
    try {
        // We expect login to succeed (exit 0) or report "already logged in" (often exit 54 with specific output)
        loginOutput = safeSystem(loginCmd, true, {54}); // Allow 54, but we'll check output
        if (loginOutput.find("Already logged in") != string::npos) {
             cout << "Export Engine: Confirmed already logged in (or login reported benign existing session). Proceeding." << endl;
        } else if (loginOutput.find("Fetching nodes") != string::npos || loginOutput.find("Login complete") != string::npos || loginOutput.empty()){ // Empty output can mean success too
             cout << "Export Engine: MEGA login successful." << endl;
        } else {
            // If exit code was 0 but output doesn't look like success, or was 54 but not "already logged in"
            // This path is less likely if safeSystem works as expected with allowedExitCodes
            // but as a safeguard if exit code 54 was for a different reason.
            // However, safeSystem will throw for non-zero unallowed codes.
            // This specific 'else' might be redundant if safeSystem's logic for allowedExitCodes handles it.
            // The main check is that if an exception wasn't thrown, we assume it's okay or handled.
             cout << "Export Engine: MEGA login command processed (exit 0 or allowed 54)." << endl;
        }
    } catch (const runtime_error &e) {
        // If safeSystem threw an error (e.g. login failed with an unallowed exit code)
        cerr << "Export Engine: Critical error during mega-login: " << e.what() << endl;
        throw; 
    }


    cout << "Export Engine: Attempting to create/verify MEGA remote directory: " << remote_dir << endl;
    string mkdirCmd = "mega-mkdir -p \"" + remote_dir + "\"";
    string mkdirOutput;
    try {
        // Allow exit code 54 for mkdir if it means "folder already exists"
        mkdirOutput = safeSystem(mkdirCmd, true, {54});
        if (mkdirOutput.find("Folder already exists") != string::npos || 
            mkdirOutput.find("Object (usually, a folder) already exists") != string::npos) {
             cout << "Export Engine: Remote directory " << remote_dir << " confirmed to already exist. Proceeding." << endl;
        } else {
             cout << "Export Engine: MEGA remote directory check/creation command processed." << endl;
        }
    } catch (const runtime_error &e) {
        string errMsg = e.what();
         // If it failed with exit 54 (which was allowed), check if the output confirms "Folder already exists"
        if (errMsg.find("exit code 54") != string::npos && 
            (errMsg.find("Folder already exists") != string::npos || 
             errMsg.find("Object (usually, a folder) already exists") != string::npos ||
             errMsg.find("Already exists") != string::npos)) { // Added generic "Already exists"
            cout << "Export Engine: Remote directory " << remote_dir << " confirmed to already exist (caught exception but stderr confirms). Proceeding." << endl;
        } else {
            cerr << "Export Engine: Critical error during mega-mkdir: " << errMsg << endl;
            throw;
        }
    }
}

int main(int argc, char* argv[]) {
    if (argc < 3) {
        cerr << "Export Engine Usage: " << argv[0] << " <local_metrics_filepath> <full_mega_remote_dir_path>\n";
        return EXIT_FAILURE;
    }
    string local_filepath = argv[1];
    string remote_mega_dir = argv[2];

    cout << "Export Engine started." << endl;
    cout << "Local file to upload: " << local_filepath << endl;
    cout << "Target MEGA directory: " << remote_mega_dir << endl;

    try {
        validateLoginAndPrepareRemoteDir(remote_mega_dir);

        string uploadCmd = "mega-put -v \"" + local_filepath + "\" \"" + remote_mega_dir + "\"";
        safeSystem(uploadCmd); // Standard error checking for put (non-zero is error)
        cout << "Export Engine: Successfully uploaded local file: " << local_filepath << " to MEGA directory: " << remote_mega_dir << endl;

        cout << "Export Engine: Attempting to delete local file: " << local_filepath << endl;
        if (remove(local_filepath.c_str()) != 0) {
            perror(("Export Engine Warning: Could not delete local file " + local_filepath).c_str());
        } else {
            cout << "Export Engine: Successfully deleted local file: " << local_filepath << endl;
        }
    } catch (const exception& e) {
        cerr << "Export Engine: FATAL ERROR: " << e.what() << endl;
        return EXIT_FAILURE;
    }
    cout << "Export Engine finished successfully." << endl;
    return EXIT_SUCCESS;
}