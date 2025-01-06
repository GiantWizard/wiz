package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os/exec"
    "fmt"
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

func formatNumber(n float64) string {
    if n >= 1000000 {
        return fmt.Sprintf("%.1fM", n/1000000)
    } else if n >= 1000 {
        return fmt.Sprintf("%.1fK", n/1000)
    }
    return fmt.Sprintf("%.0f", n)
}

func main() {
    funcMap := template.FuncMap{
        "formatNumber": formatNumber,
    }

    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        cmd := exec.Command("python3", "bz.py")
        output, err := cmd.Output()
        if err != nil {
            log.Printf("Error running Python script: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        var profits []CraftProfit
        err = json.Unmarshal(output, &profits)
        if err != nil {
            log.Printf("Error parsing JSON: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        tmpl, err := template.New("wiz.html").Funcs(funcMap).ParseFiles("wiz.html")
        if err != nil {
            log.Printf("Error parsing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        err = tmpl.Execute(w, profits)
        if err != nil {
            log.Printf("Error executing template: %v", err)
            return
        }
    })

    log.Println("Server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}