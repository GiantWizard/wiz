flowchart TD
    A[Start application] --> B[Initialize async channels]
    B --> C[Spawn API poller task]
    B --> F[Spawn export manager task]

    %% API Poller Task
    C --> G[Fetch data from API]
    G --> H[Heartbeat log after data is fetched]
    H --> I[Initialize and pass data to processing task]
    I --> K[Initialize data processor task]

    I --> J[Wait for 20s timer to tick]
    J --> G

    %% Data Processor Task
    K --> L{New data available?}
    L -->|Yes| M[Record event and start routine mini-processing]
    L -->|No| N[Record event]
    L -->|Error| O[Record event and log API error]

    M --> P[Is first snapshot?]
    P --> |Yes| Q[Initialize product states]

    %% Product States Subgraph
    subgraph Product states
      direction TB
      Q_START(For each product) --> Q_A[Create a product states field with the following states]
      Q_A --> Q_B[Initialize snapshots count, windows processed, and starting timestamp]
      Q_B --> Q_C[Initialize price list with current price in it]
      Q_C --> Q_D["Initialize MovingWeek deltas list (should have no negative numbers and should be empty on initialization)"]
      Q_D --> Q_E["Initialize summaries deltas list (list of dictionaries with events as the dictionaries, and the amount of orders dissapeared/modified and the size of those changes as the keys)"]
      Q_E --> Q_F["Initialize new demand/supply deltas (size and amount of new offers)"]

    end
    
    
    Q --> Q_START
    Q_F --> G
    P --> |No| R[Update product states]

    %% Update Logic Subgraph
    subgraph Update Logic
        direction TB
        R_START(For each product in snapshot) --> R_A{Compare with stored state}
        R_A -->|Changed| R_B{What changed?}
        R_A -->|Unchanged| R_C[Append the current price and both the MovingWeek and summaries delta to their respective lists in the product states]
        R_C --> R_G
    
        R_B -->|Price| R_D[Append new price to the price list in product states]
        R_B -->|MovingWeek| R_E[Subtract previous snapshot MovingWeek from current snapshot MovingWeek, then append this delta to the MovingWeek list in product states]
        R_B -->|Summaries| R_F[Compare and mark changes from previous summaries, the append the amount of orders that dissapeared/appeared orders and the size of those changes]
        R_D --> R_G[Update process finished]
        R_E --> R_G
        R_F --> R_G
        
    end
    
    %% Connecting the main flow to the subgraph
    R --> R_START
    R_G --> G
