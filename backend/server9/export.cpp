// file: server9/export.cpp

#include <iostream>
#include <fstream>
#include <cstdlib>
#include <cstdio>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>
#include <sys/wait.h>

using namespace std;

// Executes a shell command and captures its output. This function is unchanged.
string safeSystem(const string& cmd, bool checkError = true, const vector<int>& allowedExitCodes = {}) {
    string effective_cmd = "env HOME=/home/appuser " + cmd;
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
 * @brief Ensures the parent directory of a full remote path exists on MEGA.
 *
 * @param full_remote_path The complete destination path, e.g., "/remote_metrics/metrics.json"
 */
void ensureRemoteParentDirExists(const string& full_remote_path) {
    size_t last_slash_idx = full_remote_path.find_last_of('/');

    // Check if there is a parent directory (i.e., not a file in the root)
    if (last_slash_idx != string::npos && last_slash_idx > 0) {
        string remote_dir = full_remote_path.substr(0, last_slash_idx);
        cout << "Export Engine: Ensuring remote directory exists: " << remote_dir << endl;
        string mkdirCmd = "mega-mkdir -p \"" + remote_dir + "\"";
        
        // We allow exit code 54, which mega-mkdir returns if the folder already exists.
        // Any other non-zero code is an error.
        safeSystem(mkdirCmd, true, {54});
        cout << "Export Engine: Remote directory check/creation processed successfully." << endl;
    }
}

int main(int argc, char* argv[]) {
    if (argc < 3) {
        cerr << "Export Engine Usage: " << argv[0] << " <local_metrics_filepath> <full_mega_remote_path>\n";
        return EXIT_FAILURE;
    }
    string local_filepath = argv[1];
    string remote_mega_path = argv[2];

    cout << "Export Engine started." << endl;
    cout << "Local file to upload: " << local_filepath << endl;
    cout << "Target MEGA path: " << remote_mega_path << endl;

    try {
        // 1. Ensure the parent directory exists on MEGA before trying to upload.
        ensureRemoteParentDirExists(remote_mega_path);

        // 2. Upload the file to the full path. This works because the parent dir is now guaranteed to exist.
        string uploadCmd = "mega-put -v \"" + local_filepath + "\" \"" + remote_mega_path + "\"";
        safeSystem(uploadCmd);
        cout << "Export Engine: Successfully uploaded " << local_filepath << " to " << remote_mega_path << endl;

        // 3. Delete the local file after the successful upload.
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