$ErrorActionPreference='Stop'
$dir=$PSScriptRoot
$exe=Join-Path $dir 'LANSend-Desktop.exe'
$rx=Join-Path $dir 'receiver.exe'
Get-Process -Name receiver -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $rx } | ForEach-Object { $_.Kill(); $_.WaitForExit() }
Copy-Item -LiteralPath $exe -Destination $rx -Force
$w=New-Object -ComObject WScript.Shell
function Link($folder,$name,$target,$arguments) {
  $link=$w.CreateShortcut((Join-Path ([Environment]::GetFolderPath($folder)) $name))
  $link.TargetPath=$target
  $link.Arguments=$arguments
  $link.WorkingDirectory=$dir
  $link.Save()
}
Link 'Programs' 'LANSend.lnk' $exe ''
Link 'SendTo' '发送到 Mac.lnk' $exe 'menu'
Link 'Startup' 'LANSend Receiver.lnk' $rx 'receive'
Start-Process -FilePath $rx -ArgumentList receive
