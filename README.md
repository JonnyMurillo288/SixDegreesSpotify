# SixDegreesSpotify (Now SixDegreesMusicBrainz)

![site image](https://github.com/JonnyMurillo288/SixDegreesSpotify/blob/main/results/Screenshot%20from%202025-11-30%2012-20-01.png)

SixDegreesSpotify (now running on **MusicBrainz data**) discovers the shortest or weighted relationship path between two musical artists.  
Given a *start* artist and a *target* artist, the system builds a collaboration graph from MusicBrainz artist-credit relationships and then finds the path using:

- **Unweighted BFS** (default)  
- **Weighted search** (planned)  


SixDegreesSpotify began as a tool for discovering relationships between any two musical artists using **Spotify’s API**. However, Spotify’s API has multiple structural limitations for building a *complete, precise collaboration graph*. The project now uses **MusicBrainz**, a fully open, community-curated database with **complete recording + artist-credit metadata**, solving the problems that blocked deeper pathfinding.

**What changed?**  
- ~~Spotify API~~ → **MusicBrainz PostgreSQL database + WS/2 API fallback**  
- Access tokens + OAuth → **Direct SQL access + no auth rate-limit bottlenecks**  
- Partial collaboration data → **Full artist-credit joins across recordings**  
- Sparse edges → **Accurate per-track collaboration edges enabling true BFS paths**

---

# Problem With Spotify (And Why MusicBrainz Fixes It)

### Problem: Spotify does *not* expose detailed collaboration edges.
Spotify does not give a true “collaboration graph.” It exposes:
- Track artists, but **not** consistent canonical IDs across regions  
- Limited pagination (50 tracks per request)  
- Heavy rate limiting  
- No way to discover *all* recordings an artist appears on, especially older/obscure releases

This meant BFS stalled, returned incomplete neighbors, or produced incorrect paths.

### Solution: MusicBrainz has complete relational metadata.
MusicBrainz provides:
- A **full relational schema** containing every recording, artist, and artist_credit entry  
- Each recording → list of participating artists  
- A stable **PostgreSQL dump** for offline querying (no rate limits, no misses)  
- Consistent global artist IDs  

Switching to MusicBrainz solved:
- Missing data  
- Incomplete collaboration edges  
- Rate-limit failures  
- BFS dead-ends  
- Incorrect or missing intermediary hops

---
## Why This Project Matters (For Recruiters & Engineers)

#### This project demonstrates experience in:

- Graph algorithms (BFS, traversal strategies, path reconstruction)
- Backend engineering in Go
- Large-scale data modeling
- Performance optimization and caching
- Building clean API interfaces
- Working with complex real-world data
- Designing systems for both technical and non-technical users
- The architecture can scale to cloud deployment using AWS RDS + EC2/Lambda + caching layers.

---
## Functionailty and Performance to Incorporate
### Backend
- Send Postgres to AWS
  - Connect to AWS DB instead of my internal DB
- Cache artists
- Intermediate showings

### Functionality
- Spotify connection to create a playlist for people
- Add Photos
- Add different Connection Types (Producers, etc.)
- Each node, add their connections from there, just one level for now

### UI
- Hover over an artist/song for more information
- Show progress

---

## Prerequisites
- Go 1.21+ (recommended)
- Spotify Developer account and app
- Network access to Spotify Web API

---

## Authentication Setup
The app uses Spotify OAuth to obtain an access token, stored locally. Do not commit secrets or tokens.

1) Create a Spotify application
- Redirect URL: `http://localhost:8392/auth`

2) Create auth config file
```bash
cp main/authConfig.sample.json main/authConfig.txt
# Edit main/authConfig.txt and fill in client_id and client_secret
```

3) Run the auth server and complete login
```bash
go run ./main/auth.go
# Open http://localhost:8392 in your browser and approve access
# The token will be written to main/authToken.txt
```

---

## CLI Usage
Run an unweighted shortest path search:
```bash
go run main.go -start "Artist A" -find "Artist B"
```

If required flags are missing, the program prints usage and exits with code 1.

Optional flags (planned):
- `-depth` (limit BFS depth)
- `-weighted` (use weighted search)
- `-verbose` (more detailed logs)

---

## Notes
- Secrets (`main/authConfig.txt`, `main/authToken.txt`) are ignored via .gitignore.
- API rate limiting and pagination enhancements are planned; current behavior may be limited.

---

## License
MIT (or project default).
