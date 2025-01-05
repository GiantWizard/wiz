package main

import (
    "html/template"
    "log"
    "net/http"
)

func funcResults() []int {
    var results []int
    x, y := 3848, 1293
    for i := 0; i < 10; i++ {
        results = append(results, x+y)
        x++
    }
    return results
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
    results := funcResults()
    tmpl, err := template.ParseFiles("templates/wiz.html")
    if err != nil {
        log.Printf("Error parsing template: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
    err = tmpl.Execute(w, results)
    if err != nil {
        log.Printf("Error executing template: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    }
}

func main() {
    http.HandleFunc("/", indexHandler)
    log.Println("Server starting on port 8081...")
    err := http.ListenAndServe(":8081", nil)
    if err != nil {
        log.Fatalf("Server failed to start: %v", err)
    }
}