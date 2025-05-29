#include <iostream>
#include <fstream>
#include <cstdlib>    // For getenv
#include <cstdio>     // For popen, pclose, fgets, remove, perror
#include <sstream>
#include <stdexcept>
#include <string>
#include <sys/wait.h> // For WIFEXITED, WEXITSTATUS, WTERMSIG (on POSIX systems)
// #include <unistd.h> // For sleep, if you re-add it

using namespace std;

// Executes a shell command and captures its output.
// Throws runtime_error on failure if checkError is true.
void safeSystem(const string& cmd, bool checkError = true) {
    string effective_cmd = cmd;
    // For all mega-* commands, prepend "env HOME=/home/appuser" to force context
    if (cmd.rfind("mega-", 0) == 0) {
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
        while (!line.empty() && (line.back() == '\n' || line.back() == '\r')) {
            line.pop_back();
        }
        output_stream << line << endl;
    }
    string cmd_output_str = output_stream.str();

    int status = pclose(pipe);
    int exit_code = -1; // Default for non-normal exit

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

    // Specific handling for MEGA exit codes if needed, e.g., 54 for "already exists" might not be an error
    // For now, any non-zero is treated as an error if checkError is true.
    if (checkError && exit_code != 0) {
        string error_msg = "Export Engine: Command [" + effective_cmd + "] failed ";
        if (exit_code != -1) { // If we have a valid exit code
            error_msg += "with exit code " + to_string(exit_code);
        } else { // If termination was due to a signal or other non-standard exit
            error_msg += "(abnormal termination, status: " + to_string(status) + ")";
        }
        if(!cmd_output_str.empty()) { // Append output if it exists
             error_msg += ". Output: " + cmd_output_str;
        }
        throw runtime_error(error_msg);
    }
}


void validateLoginAndPrepareRemoteDir(const string& remote_dir) {
    cout << "Export Engine: Attempting to clear previous MEGA session state..." << endl;
    safeSystem("mega-logout", false);      // Try explicit logout first
    safeSystem("mega-ipc killserver", false); // Try to kill any lingering server
    safeSystem("mega-ipc wipeme", false);   // Wipe local IPC state
    // Consider a brief sleep if server needs time to fully shut down before restarting
    // sleep(1); // Requires #include <unistd.h> for POSIX sleep

    const char* email_env = getenv("MEGA_EMAIL");
    const char* password_env = getenv("MEGA_PWD");

    string email = email_env ? string(email_env) : "";
    string password = password_env ? string(password_env) : "";

    if (email.empty() || password.empty()) {
        throw runtime_error("Missing environment variables for login (MEGA_EMAIL or MEGA_PWD not found/empty)");
    }
    cout << "Export Engine: Attempting MEGA login for user: " << email << endl;
    string loginCmd = "mega-login \"" + email + "\" \"" + password + "\"";
    try {
        safeSystem(loginCmd); // checkError is true by default
        cout << "Export Engine: MEGA login successful." << endl;
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        // MEGAcmd exit code 54 for login can mean "Already logged in"
        if (errMsg.find("Already logged in") != string::npos || 
            (errMsg.find("exit code 54") != string::npos && errMsg.find("Already logged in") != string::npos) ) { // Check for specific stderr message too
            cout << "Export Engine: Already logged in or login reported benign existing session. Proceeding." << endl;
        } else {
            cerr << "Export Engine: Critical error during mega-login: " << errMsg << endl;
            throw; // Re-throw if it's a different, critical error
        }
    }

    cout << "Export Engine: Attempting to create/verify MEGA remote directory: " << remote_dir << endl;
    try {
        safeSystem("mega-mkdir -p \"" + remote_dir + "\"");
        cout << "Export Engine: MEGA remote directory check/creation command executed for " << remote_dir << endl;
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        // MEGAcmd exit code 54 for mkdir can mean "Folder already exists".
        // Check the actual output string for confirmation as exit codes can be overloaded.
        if (errMsg.find("Object (usually, a folder) already exists") != string::npos ||
            errMsg.find("Folder already exists") != string::npos || // Check for this specific string
            (errMsg.find("exit code 54") != string::npos && (errMsg.find("Folder already exists") != string::npos || errMsg.find("Object (usually, a folder) already exists") != string::npos )) ||
            errMsg.find("EEXIST") != string::npos || // General system error for "exists"
            errMsg.find("error code: -9") != string::npos ) { // MEGA specific internal code for "already exists"
            cout << "Export Engine: Remote directory " << remote_dir << " likely already exists. Proceeding." << endl;
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
        safeSystem(uploadCmd);
        cout << "Export Engine: Successfully uploaded local file: " << local_filepath << " to MEGA directory: " << remote_mega_dir << endl;

        cout << "Export Engine: Attempting to delete local file: " << local_filepath << endl;
        if (remove(local_filepath.c_str()) != 0) {
            perror(("Export Engine Warning: Could not delete local file " + local_filepath).c_str());
        } else {
            cout << "Export Engine: Successfully deleted local file: " << local_filepath << endl;
        }
        // Optional: Logout after successful operations
        // safeSystem("mega-logout", false);
    } catch (const exception& e) {
        cerr << "Export Engine: FATAL ERROR: " << e.what() << endl;
        // Attempt logout on error too, but don't let its failure mask the original error.
        // safeSystem("mega-logout", false);
        return EXIT_FAILURE;
    }
    cout << "Export Engine finished successfully." << endl;
    return EXIT_SUCCESS;
}