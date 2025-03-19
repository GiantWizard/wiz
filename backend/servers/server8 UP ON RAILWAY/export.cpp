#include <iostream>
#include <fstream>
#include <chrono>
#include <ctime>
#include <cstdlib>
#include <csignal>
#include <unistd.h>
#include <cstring>
#include <sstream>
#include <stdexcept>

using namespace std;

volatile sig_atomic_t stop_flag = 0;

void signal_handler(int signum) {
    stop_flag = 1;
}

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
}

void validateLogin(const string& remote_dir) {
    // Clear existing sessions
    safeSystem("mega-whoami", false);
    safeSystem("mega-logout", false);
    
    // Login with credentials from Railway env variables
    const char* username = getenv("MEGA_USERNAME");
    const char* password = getenv("MEGA_PASSWORD");
    if (!username || !password) {
        throw runtime_error("Missing environment variables for login");
    }
    
    string loginCmd = "mega-login -v " + string(username) + " " + string(password);
    safeSystem(loginCmd);

    // Create target folder once.
    // Catch the error if the folder already exists.
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

string get_current_timestamp() {
    auto now = chrono::system_clock::now();
    time_t timeNow = chrono::system_clock::to_time_t(now);
    tm local_time = *localtime(&timeNow);
    
    char buffer[80];
    strftime(buffer, sizeof(buffer), "%Y-%m-%d_%H-%M-%S", &local_time);
    return string(buffer);
}

void upload_timestamped_file(const string& remote_dir) {
    string timestamp = get_current_timestamp();
    string filename = "timestamp_" + timestamp + ".txt";
    
    // Create file with timestamp
    ofstream file(filename);
    if (!file) throw runtime_error("Failed to create file: " + filename);
    file << "Timestamp: " << timestamp << endl;
    file.close();

    try {
        safeSystem("mega-put \"" + filename + "\" \"" + remote_dir + "\"");
        cout << "Uploaded: " << filename << " to " << remote_dir << endl;
        // Delete local file after successful upload
        if (remove(filename.c_str()) != 0) {
            cerr << "Warning: Could not delete file: " << filename << endl;
        } else {
            cout << "Deleted local file: " << filename << endl;
        }
    }
    catch (...) {
        // Remove file on error and rethrow exception
        remove(filename.c_str());
        throw;
    }
}

int main() {
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
    
    const string remote_dir = "/TimestampArchive";

    try {
        if (!getenv("MEGA_USERNAME") || !getenv("MEGA_PASSWORD")) {
            cerr << "Error: Set credentials first using Railway environment variables.\n";
            return EXIT_FAILURE;
        }

        // One-time setup: login and create folder
        validateLogin(remote_dir);

        // Main upload loop
        while (!stop_flag) {
            upload_timestamped_file(remote_dir);
            
            // 3-second interval with interrupt check
            for (int i = 0; i < 3 && !stop_flag; ++i) {
                sleep(1);
            }
        }

        safeSystem("mega-logout");
        cout << "Graceful shutdown completed\n";
        return EXIT_SUCCESS;
    }
    catch (const exception& e) {
        safeSystem("mega-logout", false);
        cerr << "\nFatal error: " << e.what() << endl;
        return EXIT_FAILURE;
    }
}
