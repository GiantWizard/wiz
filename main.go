package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os/exec"
    "strings"
    "os"
    "fmt"
    "bytes"
)

// Add this function before main()
func getItemIdFromName(urlName string) string {
    // Read and parse data.json
    data, err := os.ReadFile("static/data.json")
    if err != nil {
        log.Printf("Error reading data.json: %v", err)
        return urlName
    }

    var items []struct {
        ID   string `json:"id"`
        Name string `json:"name"`
    }
    if err := json.Unmarshal(data, &items); err != nil {
        log.Printf("Error parsing data.json: %v", err)
        return urlName
    }

    // Convert URL name to lowercase for comparison
    urlName = strings.ToLower(urlName)
    urlName = strings.ReplaceAll(urlName, "-", " ")

    // Find matching item
    for _, item := range items {
        itemNameNormalized := strings.ToLower(item.Name)
        itemNameNormalized = strings.ReplaceAll(itemNameNormalized, "-", " ")
        if itemNameNormalized == urlName {
            return item.ID
        }
    }

    return urlName // Fallback to using the URL name as the ID
}

func main() {
    // Define template functions
    funcMap := template.FuncMap{
        "formatNumber": func(n float64) string {
            if n >= 1000000 {
                return fmt.Sprintf("%.1fM", n/1000000)
            } else if n >= 1000 {
                return fmt.Sprintf("%.1fK", n/1000)
            }
            return fmt.Sprintf("%.1f", n)
        },
    }

    // Serve static files
    http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    // Recipe handler
    http.HandleFunc("/recipe/", func(w http.ResponseWriter, r *http.Request) {
        urlName := strings.TrimPrefix(r.URL.Path, "/recipe/")
        if urlName == "" {
            http.Error(w, "No item provided", http.StatusBadRequest)
            return
        }

        // Convert URL-safe name back to item ID
        itemId := getItemIdFromName(urlName)

        // Run recipe.py with the item ID
        cmd := exec.Command("python3", "recipe.py")
        cmd.Env = append(os.Environ(), fmt.Sprintf("ITEM_ID=%s", itemId))
        
        output, err := cmd.CombinedOutput()
        if err != nil {
            log.Printf("Error running recipe.py: %v", err)
            http.Error(w, "Error processing recipe", http.StatusInternalServerError)
            return
        }

        // Find the start of the JSON data
        jsonStart := bytes.Index(output, []byte("{"))
        if jsonStart == -1 {
            http.Error(w, "Invalid recipe data", http.StatusInternalServerError)
            return
        }
        jsonData := output[jsonStart:]

        // Parse the recipe data
        var recipeData map[string]interface{}
        if err := json.Unmarshal(jsonData, &recipeData); err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Execute the recipe template
        tmpl, err := template.New("recipe.html").Funcs(funcMap).ParseFiles("static/recipe.html")
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        tmpl.Execute(w, recipeData)
    })

    // Main page handler
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.NotFound(w, r)
            return
        }

        // Run list.py to get the list of items
        cmd := exec.Command("python3", "list.py")
        output, err := cmd.CombinedOutput()
        if err != nil {
            log.Printf("Error running list.py: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Parse the JSON output
        var items []map[string]interface{}
        if err := json.Unmarshal(output, &items); err != nil {
            log.Printf("Error parsing JSON: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Execute template with the items data
        tmpl, err := template.New("wiz.html").Funcs(funcMap).ParseFiles("static/wiz.html")
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        tmpl.Execute(w, items)
    })

    log.Println("Server starting at http://localhost:8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatal(err)
    }
}