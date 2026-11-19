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
$appid='HKCU:\Software\Classes\AppUserModelId\LANSend.Receipts'
New-Item -Path $appid -Force | Out-Null
New-ItemProperty -Path $appid -Name DisplayName -Value 'LANSend' -Force | Out-Null
New-ItemProperty -Path $appid -Name CustomActivator -Value '{4A4A1C9A-63B7-436A-8C13-1EC4E72759EF}' -Force | Out-Null
$scheme='HKCU:\Software\Classes\lansend-reveal'
New-Item -Path "$scheme\shell\open\command" -Force | Out-Null
Set-Item -Path $scheme -Value 'URL:LANSend Received File'
New-ItemProperty -Path $scheme -Name 'URL Protocol' -Value '' -Force | Out-Null
Set-Item -Path "$scheme\shell\open\command" -Value ('"{0}" reveal "%1"' -f $rx)
Start-Process -FilePath $rx -ArgumentList receive
