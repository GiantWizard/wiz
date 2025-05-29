#include <iostream>
#include <fstream>
#include <cstdlib>    // For getenv
#include <cstdio>     // For popen, pclose, fgets, remove, perror
#include <sstream>
#include <stdexcept>
#include <string>
#include <sys/wait.h> // For WIFEXITED, WEXITSTATUS, WTERMSIG (on POSIX systems)

// Using namespace std; // It's often better to qualify (std::cout) or use specific 'using' declarations.
// For brevity in this example, we'll keep it, but be mindful in larger projects.
using namespace std;

// Executes a shell command and captures its output.
// Throws runtime_error on failure if checkError is true.
void safeSystem(const string& cmd, bool checkError = true) {
    cout << "Export Engine Executing: " << cmd << endl; // Use endl for flushing

    // Ensure mega-cmd-server has a chance to start/stabilize, especially after wipeme
    // This is a bit of a hack; a proper solution might involve checking server status.
    if (cmd.find("mega-login") != string::npos || cmd.find("mega-whoami") != string::npos) {
        // sleep(1); // Consider a small delay if server startup is an issue; requires <unistd.h>
    }

    FILE* pipe = popen(cmd.c_str(), "r"); // Capture only stdout; stderr is mixed by shell usually or handled by mega tools
    if (!pipe) {
        throw runtime_error("Failed to execute popen for command: " + cmd);
    }

    stringstream output_stream;
    char buffer[256];
    string line;

    // Read pipe output line by line
    while (fgets(buffer, sizeof(buffer), pipe) != NULL) {
        line = buffer;
        // Trim trailing newline characters for cleaner multi-line logging
        while (!line.empty() && (line.back() == '\n' || line.back() == '\r')) {
            line.pop_back();
        }
        output_stream << line << endl; // Add back a single endl for consistent logging
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
        // Only print "Command Output" header if there is output
        cout << "Export Engine Command Output:\n" << cmd_output_str << endl;
    } else {
        cout << "Export Engine: Command produced no direct output to stdout via pipe." << endl;
    }

    if (checkError && exit_code != 0) { // Check specific exit code if normally exited
        string error_msg = "Export Engine: Command [" + cmd + "] failed ";
        if (exit_code != -1) { // If we have a valid exit code
            error_msg += "with exit code " + to_string(exit_code);
        } else { // If termination was due to a signal or other non-standard exit
            error_msg += "(abnormal termination, status: " + to_string(status) + ")";
        }
        if (!cmd_output_str.empty()) { // Append output if it exists
            error_msg += ". Output: " + cmd_output_str;
        }
        throw runtime_error(error_msg);
    }
}


// Logs in to MEGA using environment variables and creates the target folder.
void validateLoginAndPrepareRemoteDir(const string& remote_dir) {
    // It's good practice to ensure we're not in an ambiguous session state.
    // These are called with checkError=false as their failure might be normal (e.g., not logged in)
    // or they might also fail due to server issues we are trying to resolve.
    safeSystem("mega-ipc wipeme", false); // Attempt to clear stale MEGAcmd server state. This can help with "Unable to connect to service"
    safeSystem("mega-whoami", false);     // Check current user, helpful for debugging, ignore error
    safeSystem("mega-logout", false);     // Logout any previous session, ignore error

    const char* email_env = getenv("MEGA_EMAIL");
    const char* password_env = getenv("MEGA_PWD");

    // Check if env variables are null or empty strings
    string email = email_env ? string(email_env) : "";
    string password = password_env ? string(password_env) : "";

    if (email.empty() || password.empty()) {
        throw runtime_error("Missing environment variables for login (MEGA_EMAIL or MEGA_PWD not found/empty)");
    }
    cout << "Export Engine: Attempting MEGA login for user: " << email << endl;
    // Quoting email and password in case they contain special characters.
    // The -v flag for mega-login is for verbose, which might be too much for regular logs.
    // Consider removing -v if logs become too noisy once working.
    string loginCmd = "mega-login \"" + email + "\" \"" + password + "\"";
    safeSystem(loginCmd); // This will throw if login fails and checkError is true (default)
    cout << "Export Engine: MEGA login command executed successfully." << endl;

    // Create target folder (ignore error if it already exists).
    cout << "Export Engine: Attempting to create/verify MEGA remote directory: " << remote_dir << endl;
    try {
        // The -p flag ensures parent directories are created if they don't exist.
        // The -v flag for mega-mkdir might also be verbose.
        safeSystem("mega-mkdir -p \"" + remote_dir + "\"");
        cout << "Export Engine: MEGA remote directory check/creation command executed for " << remote_dir << endl;
    } catch (const runtime_error &e) {
        string errMsg = e.what();
        // More robust check for "already exists" or similar non-fatal errors for mkdir
        // MEGAcmd error messages for "already exists" can vary. EEXIST is a common system error.
        // Some MEGAcmd versions might return specific error codes like -9 (Object (usually, a folder) already exists).
        if (errMsg.find("Object (usually, a folder) already exists") != string::npos ||
            errMsg.find("EEXIST") != string::npos ||
            errMsg.find("error code: -9") != string::npos ||
            errMsg.find("Already exists") != string::npos ) {
            cout << "Export Engine: Remote directory " << remote_dir << " likely already exists. Proceeding." << endl;
        } else {
            // If it's a different error, it might be critical.
            cerr << "Export Engine: Critical error during mega-mkdir: " << errMsg << endl;
            throw; // Re-throw if it's a different, potentially critical error
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

        // Export the metrics file to the remote directory.
        // The -v flag for mega-put is for verbose output.
        string uploadCmd = "mega-put -v \"" + local_filepath + "\" \"" + remote_mega_dir + "\"";
        safeSystem(uploadCmd); // This will throw if upload fails
        cout << "Export Engine: Successfully uploaded local file: " << local_filepath << " to MEGA directory: " << remote_mega_dir << endl;

        // Delete the local file after a successful upload.
        cout << "Export Engine: Attempting to delete local file: " << local_filepath << endl;
        if (remove(local_filepath.c_str()) != 0) {
            // perror provides more system-specific error details for `remove`
            perror(("Export Engine Warning: Could not delete local file " + local_filepath).c_str());
            // This is a warning, not a fatal error for the export process itself if upload succeeded.
        } else {
            cout << "Export Engine: Successfully deleted local file: " << local_filepath << endl;
        }
        // Optional: Logout after operations.
        // safeSystem("mega-logout", false); // ignore error if logout fails
    } catch (const exception& e) {
        cerr << "Export Engine: FATAL ERROR: " << e.what() << endl;
        // Attempt logout on error too, but don't let its failure mask the original error.
        // safeSystem("mega-logout", false);
        return EXIT_FAILURE;
    }
    cout << "Export Engine finished successfully." << endl;
    return EXIT_SUCCESS;
}