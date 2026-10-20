@echo off
setlocal

where go >nul 2>nul
if errorlevel 1 (
  echo [ERROR] 未找到 go 命令。请安装 Go 1.22+ 并加入 PATH。
  exit /b 1
)

if not exist bin mkdir bin

set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui -s -w" -o bin\lantransfer.exe .
if errorlevel 1 (
  echo [ERROR] 编译失败。
  exit /b 1
)

echo [OK] 已生成 bin\lantransfer.exe
for %%F in (bin\lantransfer.exe) do echo      大小: %%~zF 字节
