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

func loadItemNames() (map[string]string, error) {
    data, err := os.ReadFile("data.json")
    if err != nil {
        return nil, err
    }

    var items map[string]struct {
        Name string `json:"name"`
    }
    if err := json.Unmarshal(data, &items); err != nil {
        return nil, err
    }

    names := make(map[string]string)
    for id, item := range items {
        names[id] = item.Name
    }
    return names, nil
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


    // Recipe handler
    http.HandleFunc("/recipe/", func(w http.ResponseWriter, r *http.Request) {
        urlName := strings.TrimPrefix(r.URL.Path, "/recipe/")
        if urlName == "" {
            http.Error(w, "No item provided", http.StatusBadRequest)
            return
        }

        // Load item names
        itemNames, err := loadItemNames()
        if err != nil {
            log.Printf("Error loading item names: %v", err)
        }

        // Run recipe.py with the item ID
        cmd := exec.Command("python3", "recipe.py")
        cmd.Env = append(os.Environ(), fmt.Sprintf("ITEM_ID=%s", urlName))
        
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

        // Add item names to the template data
        templateData := map[string]interface{}{
            "recipe": recipeData,
            "names": itemNames,
        }

        // Execute the recipe template
        tmpl, err := template.New("recipe.html").Funcs(funcMap).ParseFiles("static/recipe.html")
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        tmpl.Execute(w, templateData)
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

        // Parse the JSON output as array
        var items []map[string]interface{}
        if err := json.Unmarshal(output, &items); err != nil {
            log.Printf("Error parsing JSON: %v", err)
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }

        // Load item names
        itemNames, err := loadItemNames()
        if err != nil {
            log.Printf("Error loading item names: %v", err)
        } else {
            // Replace item IDs with names
            for _, item := range items {
                if id, ok := item["item_id"].(string); ok {
                    if name, exists := itemNames[id]; exists {
                        item["name"] = name
                    }
                }
            }
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