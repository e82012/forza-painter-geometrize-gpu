@echo off
title Geometrize GPU Web UI Server
echo ===================================================
echo   Geometrize GPU Web UI Server is starting...
echo ===================================================
echo.
echo [*] Opening http://localhost:8080 in default browser...
start http://localhost:8080
echo [*] Starting Node.js Web Server...
echo [*] Press Ctrl + C to stop the server.
echo.
node server.js
pause
