package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os/exec"
    "fmt"
    "strings"
    "sync"
    "time"
)

type CraftProfit struct {
    ItemId        string  `json:"item_id"`
    Name          string  `json:"name"`
    Profit        float64 `json:"profit"`
    ProfitPercent float64 `json:"profit_percent"`
    CraftingCost  float64 `json:"crafting_cost"`
    SellPrice     float64 `json:"sell_price"`
    CoinsPerHour  float64 `json:"coins_per_hour"`
}

type RecipeData struct {
    ItemName string
}

// Cache structure
var (
    profitsCache []CraftProfit
    cacheMutex   sync.RWMutex
    lastUpdate   time.Time
)

func formatNumber(n float64) string {
    if n >= 1000000 {
        return fmt.Sprintf("%.1fM", n/1000000)
    } else if n >= 1000 {
        return fmt.Sprintf("%.1fK", n/1000)
    }
    return fmt.Sprintf("%.0f", n)
}

func updateCache() error {
    cmd := exec.Command("python3", "bz.py")
    output, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("error running Python script: %v", err)
    }

    var profits []CraftProfit
    err = json.Unmarshal(output, &profits)
    if err != nil {
        return fmt.Errorf("error parsing JSON: %v", err)
    }

    cacheMutex.Lock()
    profitsCache = profits
    lastUpdate = time.Now()
    cacheMutex.Unlock()

    return nil
}

func main() {
    // Initial cache update
    if err := updateCache(); err != nil {
        log.Fatal(err)
    }

    // Start background cache update
    go func() {
        for {
            time.Sleep(1 * time.Minute)
            if err := updateCache(); err != nil {
                log.Printf("Cache update error: %v", err)
            }
        }
    }()

    funcMap := template.FuncMap{
        "formatNumber": formatNumber,
    }

    // Main page handler
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.NotFound(w, r)
            return
        }

        cacheMutex.RLock()
        profits := profitsCache
        cacheMutex.RUnlock()

        tmpl, err := template.New("wiz.html").Funcs(funcMap).ParseFiles("wiz.html")
        if err != nil {
            log.Printf("Error parsing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        err = tmpl.Execute(w, profits)
        if err != nil {
            if !strings.Contains(err.Error(), "broken pipe") {
                log.Printf("Error executing template: %v", err)
            }
            return
        }
    })

    // Simple recipe handler
    http.HandleFunc("/recipe/", func(w http.ResponseWriter, r *http.Request) {
        itemId := strings.TrimPrefix(r.URL.Path, "/recipe/")
        
        recipeData := RecipeData{
            ItemName: itemId,
        }

        tmpl, err := template.New("recipe.html").Funcs(funcMap).ParseFiles("recipe.html")
        if err != nil {
            log.Printf("Error parsing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        err = tmpl.Execute(w, recipeData)
        if err != nil {
            if !strings.Contains(err.Error(), "broken pipe") {
                log.Printf("Error executing template: %v", err)
            }
            return
        }
    })

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}