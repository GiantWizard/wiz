#include <iostream>
#include <fstream>
#include <cstdlib>    // For getenv
#include <cstdio>     // For popen, pclose, fgets, remove, perror
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>
#include <sys/wait.h> // For WIFEXITED, WEXITSTATUS, WTERMSIG (on POSIX systems)

using namespace std;

// Executes a shell command and captures its output.
// This function remains unchanged as it's a solid utility.
string safeSystem(const string& cmd, bool checkError = true, const vector<int>& allowedExitCodes = {}) {
    string effective_cmd = cmd;
    // Prepend HOME for all mega commands to ensure it uses the shared session.
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
    while (fgets(buffer, sizeof(buffer), pipe) != NULL) {
        output_stream << buffer;
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
        cout << "Export Engine: Command produced no direct output to stdout." << endl;
    }

    bool isAllowedExitCode = false;
    if (exit_code != -1) {
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
             error_msg += ". Output was: " + cmd_output_str;
        }
        throw runtime_error(error_msg);
    }
    return cmd_output_str;
}

/**
 * @brief Ensures the remote MEGA directory exists, creating it if necessary.
 *
 * This function is the simplified replacement for the old `validateLoginAndPrepareRemoteDir`.
 * It no longer handles login/logout. It only performs `mega-mkdir -p`, assuming
 * a valid session is already active.
 *
 * @param remote_dir The full path of the remote directory on MEGA.
 */
void ensureRemoteDirExists(const string& remote_dir) {
    cout << "Export Engine: Ensuring MEGA remote directory exists: " << remote_dir << endl;
    string mkdirCmd = "mega-mkdir -p \"" + remote_dir + "\"";

    try {
        // We allow exit code 54 because `mega-mkdir -p` returns it if the folder already exists.
        string mkdirOutput = safeSystem(mkdirCmd, true, {54});
        if (mkdirOutput.find("already exist") != string::npos) {
             cout << "Export Engine: Remote directory " << remote_dir << " confirmed to already exist." << endl;
        } else {
             cout << "Export Engine: Remote directory check/creation command processed successfully." << endl;
        }
    } catch (const runtime_error &e) {
        // This catch block handles cases where the command fails with an unallowed exit code,
        // or if it exits with 54 but for a different reason than "already exists".
        // We add a check here to be robust.
        string errMsg = e.what();
        if (errMsg.find("exit code 54") != string::npos &&
            (errMsg.find("already exist") != string::npos ||
             errMsg.find("Object (usually, a folder) already exists") != string::npos)) {
            cout << "Export Engine: Remote directory " << remote_dir << " confirmed to already exist (exception was benign)." << endl;
        } else {
            cerr << "Export Engine: Critical error during mega-mkdir: " << errMsg << endl;
            throw; // Re-throw the critical error.
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
        // CRUCIAL CHANGE: Call the new, simple function that ONLY creates the directory.
        // All login/logout/killserver logic has been removed from this process.
        ensureRemoteDirExists(remote_mega_dir);

        // This part remains the same: upload the file.
        string uploadCmd = "mega-put -v \"" + local_filepath + "\" \"" + remote_mega_dir + "\"";
        safeSystem(uploadCmd); // Use standard error checking (non-zero is an error).
        cout << "Export Engine: Successfully uploaded " << local_filepath << " to " << remote_mega_dir << endl;

        // This part remains the same: delete the local file after successful upload.
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