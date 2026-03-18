╔──────────────────────────────────────────────────────────────────────────────╗
│ OSI (Options Symbology Initiative) Symbol Format                             │
│                                                                              │
│   SSSSSSYYMMDDCPPPPPPPP                                                      │
│   ├─┬──┘├─┬──┘│├──┬───┘                                                      │
│   │ │   │ │   ││  │                                                          │
│   │ │   │ │   ││  └─ strike price × 1000, zero-padded                        │
│   │ │   │ │   │└──── C = call, P = put                                       │
│   │ │   │ │   └───── expiration day                                          │
│   │ │   │ └───────── expiration month                                        │
│   │ │   └─────────── expiration year (two digit)                             │
│   │ └─────────────── underlying symbol, left-justified, space-padded to 6    │
│   │                                                                          │
│   Examples:                                                                  │
│                                                                              │
│     AAPL  260417C00200000  AAPL $200   call exp 2026-04-17                   │
│     JPST  260619P00050500  JPST $50.50 put  exp 2026-06-19                   │
│                                                                              │
╚──────────────────────────────────────────────────────────────────────────────╝
