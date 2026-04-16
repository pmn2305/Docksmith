Docksmith 🐳⚙️
A minimal Docker-like container runtime built for learning purposes.

📌 Overview
Docksmith is a lightweight container runtime that mimics core Docker concepts such as:

Filesystem isolation using chroot
Process isolation using Linux namespaces
Image building with layered caching
Basic command execution inside containers

This project is for educational use only and does not provide production-grade security.
🚀 Features
Build images from a simple Dockerfile-like format
Layer caching for faster rebuilds
Isolated container execution (PID, UTS, mount namespaces)
Environment variable and working directory support
Command override at runtime
🛠️ Tech Stack
Go (Golang)
Linux namespaces (CLONE_NEW*)
chroot for filesystem isolation
📂 Project Structure
builder/     # Image build logic  
runtime/     # Container execution  
parser/      # Dockerfile parsing  
cache/       # Layer caching  
image/       # Image management  
▶️ Usage
1. Build an image
./docksmith build <path-to-dockerfile>
2. Run a container
./docksmith run <image> [command]

🎯 Purpose
This project was built as a course mini-project to understand how container runtimes like Docker work under the hood.

📎 Inspiration
Docker
Linux container primitives# Docksmith
