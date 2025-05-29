#include <iostream>
#include <fstream>
#include <cstdlib>    // For getenv
#include <cstdio>     // For popen, pclose, fgets, remove, perror
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>
#include <sys/wait.h> // For WIFEXITED, WEXITSTATUS, WTERMSIG (on POSIX systems)
// #include <unistd.h> // For sleep

using namespace std;

// Executes a shell command and captures its output.
// Allows specifying a vector of exit codes that should NOT be treated as errors.
void safeSystem(const string& cmd, bool checkError = true, const vector<int>& allowedExitCodes = {}) {
    string effective_cmd = cmd;
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
    for (int allowed_code : allowedExitCodes) {
        if (exit_code == allowed_code) {
            isAllowedExitCode = true;
            break;
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
             error_msg += ". Output: " + cmd_output_str;
        }
        throw runtime_error(error_msg);
    }
}

void validateLoginAndPrepareRemoteDir(const string& remote_dir) {
    cout << "Export Engine: Attempting to clear previous MEGA session state..." << endl;
    safeSystem("mega-logout", false);      // Try explicit logout first. checkError=false means we don't care if it fails (e.g. not logged in)
    safeSystem("mega-ipc killserver", false); // Try to kill any lingering server
    safeSystem("mega-ipc wipeme", false);   // Wipe local IPC state

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
        // Allow exit code 54 for login if it means "already logged in"
        safeSystem(loginCmd, true, {54}); 
        cout << "Export Engine: MEGA login command processed." << endl;
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        // If it failed with exit 54 AND the output confirms "Already logged in", it's fine.
        if (errMsg.find("exit code 54") != string::npos && errMsg.find("Already logged in") != string::npos) {
            cout << "Export Engine: Confirmed already logged in. Proceeding." << endl;
        } else {
            // Any other error during login is critical
            cerr << "Export Engine: Critical error during mega-login: " << errMsg << endl;
            throw;
        }
    }


    cout << "Export Engine: Attempting to create/verify MEGA remote directory: " << remote_dir << endl;
    string mkdirCmd = "mega-mkdir -p \"" + remote_dir + "\"";
    try {
        // Allow exit code 54 for mkdir if it means "folder already exists"
        safeSystem(mkdirCmd, true, {54});
        cout << "Export Engine: MEGA remote directory check/creation command processed." << endl;
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        // If it failed with exit 54 AND the output confirms "Folder already exists", it's fine.
        if (errMsg.find("exit code 54") != string::npos && 
            (errMsg.find("Folder already exists") != string::npos || errMsg.find("Object (usually, a folder) already exists") != string::npos)) {
            cout << "Export Engine: Remote directory " << remote_dir << " confirmed to already exist. Proceeding." << endl;
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
        safeSystem(uploadCmd); // Standard error checking for put
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