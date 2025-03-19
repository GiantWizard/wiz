#include <iostream>
#include <cstdlib>
#include <unistd.h>     // for usleep
#include <ctime>

// --- Include the MEGA SDK headers ---
// Adjust these include paths as needed for your environment.
#include "megaclient.h"
#include "megaapp.h"
#include "fileaccess.h"   // Provides PosixFileAccess
#include "httpio.h"       // Provides CurlHttpIO
#include "prngen.h"       // Provides SimplePrnGen
#include "symmcipher.h"   // Provides CryptoSymmCipher
#include "asymmcipher.h"  // Provides RSAAsymmCipher

// --- Our MEGA SDK callback handler ---
class MyMegaApp : public MegaApp {
public:
    bool loggedIn;
    bool nodesFetched;
    bool folderCreated;
    MegaClient* client;

    MyMegaApp() : loggedIn(false), nodesFetched(false), folderCreated(false), client(nullptr) {}

    // Callback for login completion.
    virtual void login_result(error e) override {
        if (e == API_OK) {
            std::cout << "MEGA SDK: Login successful." << std::endl;
            loggedIn = true;
            // After login, fetch the node tree.
            client->fetchnodes();
        }
        else {
            std::cerr << "MEGA SDK: Login failed, error: " << e << std::endl;
            std::exit(1);
        }
    }

    // Callback for fetchnodes() completion.
    virtual void fetchnodes_result(MegaClient* client, error e) override {
        if (e == API_OK) {
            std::cout << "MEGA SDK: Nodes fetched successfully." << std::endl;
            nodesFetched = true;
            // Now create the folder.
            createFolder(client);
        }
        else {
            std::cerr << "MEGA SDK: Fetch nodes failed, error: " << e << std::endl;
            std::exit(1);
        }
    }

    // Callback for node updates (for logging purposes).
    virtual void node_updated(MegaClient* /*client*/, Node** /*nodes*/, int count) override {
        std::cout << "MEGA SDK: node_updated() received for " << count << " node(s)." << std::endl;
    }

    // Callback for putnodes() completion.
    virtual void putnodes_result(MegaClient* /*client*/, error e) override {
        if (e == API_OK) {
            std::cout << "MEGA SDK: Folder creation confirmed." << std::endl;
            folderCreated = true;
        }
        else {
            std::cerr << "MEGA SDK: Folder creation failed, error: " << e << std::endl;
            std::exit(1);
        }
    }

    // Helper to create a folder using putnodes().
    void createFolder(MegaClient* client) {
        NewNode newNode;
        newNode.type = FOLDERNODE;  // Folder node type defined in the SDK

        // Use the first root node as parent.
        if (client->rootnodes && client->rootnodes[0]) {
            newNode.parent = client->rootnodes[0]->nodehandle;
        }
        else {
            std::cerr << "MEGA SDK: No valid root node available." << std::endl;
            std::exit(1);
        }
        newNode.name = "ExportedFolder";  // Name of the folder to create
        newNode.attrstring = nullptr;     // In production, set proper attributes and encryption keys.
        newNode.key = nullptr;
        std::cout << "MEGA SDK: Creating folder 'ExportedFolder'..." << std::endl;
        client->putnodes(newNode.parent, &newNode, 1);
    }
};

//
// Main function: Log in, fetch nodes, and create a folder
//
int main(int argc, char* argv[]) {
    if (argc < 4) {
        std::cerr << "Usage: exporter <json> <username> <password>" << std::endl;
        return 1;
    }
    
    std::string json_msg(argv[1]);   // For this example, JSON is not parsed further.
    std::string username(argv[2]);
    std::string password(argv[3]);
    
    std::cout << "C++ Exporter: Received JSON: " << json_msg << std::endl;
    std::cout << "C++ Exporter: Using username: " << username << std::endl;
    std::cout << "C++ Exporter: Attempting to export folder to mega.nz..." << std::endl;
    
    // --- Step 2: Create real SDK implementations ---
    PosixFileAccess fileAccess;    // Real file access using POSIX calls.
    CurlHttpIO httpIO;             // Real HTTP I/O using cURL.
    SimplePrnGen prng;             // Secure PRNG.
    CryptoSymmCipher symmCipher;   // AES-based symmetric cipher.
    RSAAsymmCipher asymmCipher;    // RSA-based asymmetric cipher.
    
    // --- Step 3: Instantiate our MegaApp handler and MegaClient ---
    MyMegaApp app;
    MegaClient* client = new MegaClient(&app, &fileAccess, &httpIO, &prng, &symmCipher, &asymmCipher);
    app.client = client;   // Save the pointer for callbacks.
    
    // --- Step 4: Log in to the MEGA account ---
    char hashBuffer[128] = {0};   // Buffer for hashed password.
    error hashErr = client->hashpw_key(password.c_str(), hashBuffer);
    if (hashErr != API_OK) {
        std::cerr << "MEGA SDK: Password hashing failed, error: " << hashErr << std::endl;
        delete client;
        return 1;
    }
    client->login(username.c_str(), hashBuffer);
    
    // --- Real Event Loop ---
    std::srand(std::time(nullptr));
    std::cout << "MEGA SDK: Entering event loop (press Ctrl+C to exit)..." << std::endl;
    while (true) {
        client->exec();
        httpIO.waitio(100);  // waitio() blocks until network I/O or timeout.
    }
    
    // Cleanup (unreachable in this example)
    delete client;
    return 0;
}
