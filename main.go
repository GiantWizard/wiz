package main

import (
    "encoding/json"
    "html/template"
    "log"
    "net/http"
    "os/exec"
    "fmt"
    "strings"
    "strconv"
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

type RecipeItem struct {
    Indent int
    Line   string
}

type RecipeData struct {
    ItemName      string
    RecipeTree    []RecipeItem
    RawItems      []RecipeItem
    TotalCost     float64
    SellingPrice  float64
    Profit        float64
    ProfitPercent float64
    CoinsPerHour  float64
    IsProfit      bool
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
        "multiply": func(a, b int) int {
            return a * b
        },
    }

    // Main page handler
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.NotFound(w, r)
            return
        }

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

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        err = tmpl.Execute(w, profits)
        if err != nil {
            if !strings.Contains(err.Error(), "broken pipe") {
                log.Printf("Error executing template: %v", err)
            }
            return
        }
    })

    // Recipe details handler
    http.HandleFunc("/recipe/", func(w http.ResponseWriter, r *http.Request) {
        itemId := strings.TrimPrefix(r.URL.Path, "/recipe/")
        
        cmd := exec.Command("python3", "bz.py", itemId)
        output, err := cmd.Output()
        if err != nil {
            log.Printf("Error running Python script for recipe: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

        lines := strings.Split(string(output), "\n")
        recipeData := RecipeData{
            ItemName: itemId,
        }

        var currentSection string
        for _, line := range lines {
            if line == "" {
                continue
            }

            if strings.Contains(line, "Recipe Tree:") {
                currentSection = "recipe"
                continue
            } else if strings.Contains(line, "Raw Items Needed") {
                currentSection = "raw"
                continue
            } else if strings.Contains(line, "Total cost:") {
                parts := strings.Split(line, ":")
                if len(parts) > 1 {
                    cost, _ := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(parts[1], ",", "")), 64)
                    recipeData.TotalCost = cost
                }
                continue
            }

            indent := 0
            for i, c := range line {
                if c != ' ' {
                    indent = i / 20
                    break
                }
            }

            if currentSection == "recipe" {
                recipeData.RecipeTree = append(recipeData.RecipeTree, RecipeItem{
                    Indent: indent,
                    Line:   strings.TrimSpace(line),
                })
            } else if currentSection == "raw" {
                recipeData.RawItems = append(recipeData.RawItems, RecipeItem{
                    Line: strings.TrimSpace(line),
                })
            }
        }

        w.Header().Set("Content-Type", "text/html; charset=utf-8")
        tmpl, err := template.New("recipe.html").Funcs(funcMap).ParseFiles("recipe.html")
        if err != nil {
            log.Printf("Error parsing template: %v", err)
            http.Error(w, "Internal Server Error", 500)
            return
        }

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
