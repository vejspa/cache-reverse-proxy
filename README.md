# Cache Reverse Proxy

## Overview
This is a lightweight reverse proxy server written in Go, with built-in caching capabilities. It caches HTTP `GET` responses to improve performance and reduce the load on the backend server. The cache is refreshed based on a configurable Time-To-Live (TTL) value.

## Features
- Reverse proxy functionality
- Caching of HTTP `GET` responses
- Configurable TTL for cache expiration
- Cache hit/miss detection via `X-Cache` headers

## Requirements
- Go 1.24 or higher
- A `.env` file containing the following variable:
  - `TTL`: Cache expiration time in hours (integer)

## Installation

1. **Clone the repository:**
   ```bash
   git clone git@github.com:vejspa/cache-reverse-proxy.git
   cd cache-reverse-proxy
   
2. **Install dependencies:**
   ```
   go mod tidy
3. **Run the reverse proxy:**
   ```
   go run main.go

## Usage
1. Reverse-Proxy listens on port 8080 requests
2. By default handles requests directed to https://dummyjson.com
3. Removes X-Forwarded-For to avoid IP spoofing
4. Inside .env store TTL value in hours.
