#include <iostream>
#include <thread>
#include <chrono>
#include "megaapi.h"

using namespace std;

// A simple listener class to handle asynchronous callbacks from the Mega SDK.
class MyMegaListener : public mega::MegaListener {
public:
    // This callback is called when a request finishes.
    void onRequestFinish(mega::MegaApi* api, mega::MegaRequest* request, mega::MegaError* e) override {
        // Check for export request callback
        if(request->getType() == mega::MegaRequest::TYPE_EXPORT) {
            if(e->getErrorCode() == mega::MegaError::API_OK) {
                cout << "Export link: " << request->getLink() << endl;
            }
            else {
                cout << "Export failed: " << e->getErrorString() << endl;
            }
        }
        // Optional: handle login and node fetching callbacks for debugging
        else if(request->getType() == mega::MegaRequest::TYPE_LOGIN) {
            if(e->getErrorCode() == mega::MegaError::API_OK) {
                cout << "Login successful." << endl;
            }
            else {
                cout << "Login failed: " << e->getErrorString() << endl;
            }
        }
        else if(request->getType() == mega::MegaRequest::TYPE_FETCH_NODES) {
            if(e->getErrorCode() == mega::MegaError::API_OK) {
                cout << "Nodes fetched successfully." << endl;
            }
            else {
                cout << "Fetch nodes failed: " << e->getErrorString() << endl;
            }
        }
    }
};

int main() {
    // Replace these with your actual Mega credentials and application information.
    const char* appKey    = "YOUR_APP_KEY";      // Your Mega application key
    const char* userAgent = "YourAppName";         // Your app's user agent
    const char* email     = "your-email@example.com";  // Your Mega account email
    const char* password  = "your-password";       // Your Mega account password

    // Create the MegaApi object. The Mega SDK will use this object to manage API calls.
    mega::MegaApi* megaApi = new mega::MegaApi(appKey, userAgent);

    // Set up our listener for asynchronous callbacks.
    MyMegaListener listener;
    megaApi->addListener(&listener);

    // Log in to your Mega account.
    cout << "Logging in..." << endl;
    megaApi->login(email, password);

    // Wait a bit for the login process to complete.
    std::this_thread::sleep_for(std::chrono::seconds(10));

    // Fetch your account's nodes (file/folder structure).
    cout << "Fetching nodes..." << endl;
    megaApi->fetchNodes();
    std::this_thread::sleep_for(std::chrono::seconds(5));

    // Get the root node of your account.
    mega::MegaNode* root = megaApi->getRootNode();
    if (!root) {
        cout << "Unable to retrieve the root node." << endl;
        delete megaApi;
        return 1;
    }

    // Retrieve the children of the root node.
    mega::MegaNodeList* children = megaApi->getChildren(root);
    if (children->size() > 0) {
        // Find the first node that is a file (and not a folder).
        mega::MegaNode* targetNode = nullptr;
        for (int i = 0; i < children->size(); i++) {
            mega::MegaNode* node = children->get(i);
            if (node->isFile()) {
                targetNode = node;
                break;
            }
        }
        if (targetNode) {
            cout << "Exporting file: " << targetNode->getName() << endl;
            // Export the node. The second parameter (true) indicates that the file should be exported (made public).
            megaApi->exportNode(targetNode, true);
        }
        else {
            cout << "No file found in the root folder." << endl;
        }
    }
    else {
        cout << "The root node has no children." << endl;
    }

    // Wait for the asynchronous export callback to be invoked.
    cout << "Waiting for export callback..." << endl;
    std::this_thread::sleep_for(std::chrono::seconds(10));

    // Clean up and release resources.
    delete children;
    delete root;
    delete megaApi;

    return 0;
}
