$ErrorActionPreference = 'Stop'
$dir = [Environment]::GetFolderPath('SendTo')
$shell = New-Object -ComObject WScript.Shell
$keep = @{}
foreach ($device in (ConvertFrom-Json $env:LANSEND_DEVICE_MENUS)) {
    $path = Join-Path $dir ($device.Name + '.lnk')
    $link = $shell.CreateShortcut($path)
    if ((Test-Path -LiteralPath $path) -and $link.Description -ne 'LANSend device') { throw "Shortcut name in use" }
    $link.Description = 'LANSend device'
    $link.TargetPath = $env:LANSEND_MENU_EXE
    $link.Arguments = 'send-to ' + $device.Hash
    $link.Save()
    $keep[$device.Name + '.lnk'] = $true
}
foreach ($link in (Get-ChildItem -LiteralPath $dir -Filter 'LANSend · *.lnk' -File)) {
    if (!$keep.ContainsKey($link.Name) -and $shell.CreateShortcut($link.FullName).Description -eq 'LANSend device') {
        Remove-Item -LiteralPath $link.FullName
    }
}
