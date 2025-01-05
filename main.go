package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os/exec"
)

type CraftProfit struct {
    ItemId        string  `json:"item_id"`
    Profit        float64 `json:"profit"`
    ProfitPercent float64 `json:"profit_percent"`
    CraftingCost  float64 `json:"crafting_cost"`
    SellPrice     float64 `json:"sell_price"`
    CoinsPerHour  float64 `json:"coins_per_hour"`
}

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        // Run Python script
        cmd := exec.Command("python3", "bz.py")
        output, err := cmd.Output()
        if err != nil {
            log.Printf("Error running Python script: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        // Parse the output into our struct
        var profits []CraftProfit
        err = json.Unmarshal(output, &profits)
        if err != nil {
            log.Printf("Error parsing JSON: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        // Parse and execute template
        tmpl, err := template.ParseFiles("project/templates/bazaar.html")
        if err != nil {
            log.Printf("Error parsing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        err = tmpl.Execute(w, profits)
        if err != nil {
            log.Printf("Error executing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }
    })

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
} 