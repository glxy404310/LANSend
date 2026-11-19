# Run in a fresh Windows desktop session with the release archive extracted.
param([Parameter(Mandatory)][string]$Executable)
$ErrorActionPreference='Stop'
$Executable=(Resolve-Path $Executable).Path
$profileDir=Join-Path ([IO.Path]::GetTempPath()) ([guid]::NewGuid().ToString())
$env:USERPROFILE="$profileDir\profile"
$env:APPDATA="$profileDir\roaming"
$env:LOCALAPPDATA="$profileDir\local"
$root="$env:APPDATA\LANSend"
$dir="$env:LOCALAPPDATA\LANSend"
$desktop="$dir\LANSend-Desktop.exe"
$receiver="$dir\receiver.exe"
$inbox="$env:USERPROFILE\Downloads\LANSend"
New-Item -Type Directory -Force $dir,$env:USERPROFILE,$env:APPDATA | Out-Null
Copy-Item $Executable $desktop
Copy-Item $Executable $receiver
Add-Type @'
using System;
using System.Collections.Generic;
using System.Text;
using System.Runtime.InteropServices;
public class ReceiptUI {
 public delegate bool Callback(IntPtr h, IntPtr data);
 [DllImport("user32")] static extern bool EnumChildWindows(IntPtr parent, Callback callback, IntPtr data);
 [DllImport("user32",CharSet=CharSet.Unicode)] static extern int GetClassName(IntPtr h,StringBuilder buf,int n);
 [DllImport("user32",CharSet=CharSet.Unicode, EntryPoint="SendMessageW")] static extern IntPtr Read(IntPtr h,int msg,IntPtr count,StringBuilder text);
 [DllImport("user32",CharSet=CharSet.Unicode)] public static extern IntPtr SendMessage(IntPtr h,int msg,IntPtr w,IntPtr l);
 [DllImport("user32",CharSet=CharSet.Unicode)] static extern IntPtr FindWindow(string cls,string title);
 public static IntPtr Receipt(){return FindWindow(null,"LANSend 收件");}
 [DllImport("user32")] public static extern bool IsWindowVisible(IntPtr h);
 public static IntPtr Child(IntPtr parent,string title,string cls){
  IntPtr found=IntPtr.Zero;
  EnumChildWindows(parent,delegate(IntPtr h,IntPtr unused){
   var c=new StringBuilder(128);GetClassName(h,c,128);
   if(cls!=null && c.ToString().ToUpperInvariant()!=cls.ToUpperInvariant())return true;
   var buf=new StringBuilder(512);Read(h,13,(IntPtr)buf.Capacity,buf);
   if(buf.ToString().StartsWith(title)){found=h;return false;}return true;
  },IntPtr.Zero);return found;
 }
}
'@
function Assert-That($truth,[string]$message){if(!$truth){throw $message}}
function Run-Engine([string[]]$argv){
 $process=Start-Process $receiver -ArgumentList $argv -PassThru
 if(!$process.WaitForExit(30000)){ $process.Kill(); throw "Timed out: $($argv[0])" }
 Assert-That ($process.ExitCode -eq 0) "Failed: $($argv[0])"
}
function Receipt { [ReceiptUI]::Receipt() }
function Dashboard {
 $p=Start-Process $desktop -PassThru
 for($i=0;$i -lt 50;$i++){Start-Sleep -Milliseconds 200;$p.Refresh();if($p.MainWindowHandle -ne 0){return $p}}
 throw 'Settings window missing'
}
function Close-TestProcess($p){if($p -and !$p.HasExited){$p.Kill();$p.WaitForExit()}}
try {
 Run-Engine @('init')
 $rx=Start-Process $receiver -ArgumentList receive -PassThru
 Run-Engine @('ready')
 # Pair only this disposable loopback identity.
 $pem=(Get-Content "$root\identity.json" -Raw | ConvertFrom-Json).Certificate
 $der=[Convert]::FromBase64String(($pem -replace '-----[^-]+-----','').Trim())
 $hash=([BitConverter]::ToString([Security.Cryptography.SHA256]::Create().ComputeHash($der))).Replace('-','').ToLowerInvariant()
 [IO.File]::WriteAllText("$root\peer.json",(@{PeerHash=$hash;Address='127.0.0.1:53318'}|ConvertTo-Json))
 $source="$profileDir\hello.txt"
 [IO.File]::WriteAllText($source,'LANSend QA')
 Run-Engine @('send',('"{0}"' -f $source),('"{0}"' -f $source))
 for($i=0;$i -lt 50;$i++){Start-Sleep -Milliseconds 100;$panel=Receipt;if([ReceiptUI]::IsWindowVisible($panel)){break}}
 $rx.Refresh()
 Write-Output "Receiver exited=$($rx.HasExited), panel handle=$panel, process window=$($rx.MainWindowHandle)"
 Assert-That ([ReceiptUI]::IsWindowVisible($panel)) 'Front window missing' 
 foreach($button in @('打开文件','打开收件夹')){
  Assert-That ([ReceiptUI]::Child($panel,$button,'BUTTON') -ne [IntPtr]::Zero) "Missing action $button"
 }
 $combo=[ReceiptUI]::Child($panel,'','COMBOBOX')
 Assert-That ($combo -ne [IntPtr]::Zero) 'File picker missing'
 Assert-That (([ReceiptUI]::SendMessage($combo,326,0,0)).ToInt32() -eq 2) 'Expected two file names in one window'
 Assert-That ([IO.File]::ReadAllText("$inbox\hello (1).txt") -eq 'LANSend QA') 'Batch content mismatch'
 [void][ReceiptUI]::SendMessage($panel,16,0,0)
 $ui=Dashboard
 $toggle=[ReceiptUI]::Child($ui.MainWindowHandle,'静默接收','BUTTON')
 Assert-That ($toggle -ne [IntPtr]::Zero) 'Missing silent switch'
 [void][ReceiptUI]::SendMessage($toggle,245,0,0)
 Assert-That (Test-Path "$root\silent-receipts") 'Silent mode not saved'
 Close-TestProcess $ui
 Close-TestProcess $rx
 $rx=Start-Process $receiver -ArgumentList receive -PassThru
 Run-Engine @('ready')
 $ui=Dashboard
 $toggle=[ReceiptUI]::Child($ui.MainWindowHandle,'静默接收','BUTTON')
 Assert-That (([ReceiptUI]::SendMessage($toggle,240,0,0)).ToInt32() -eq 1) 'Setting lost after restart'
 Run-Engine @('send',('"{0}"' -f $source))
 Start-Sleep -Milliseconds 500
 Assert-That (![ReceiptUI]::IsWindowVisible((Receipt))) 'Unexpected silent popup'
 Assert-That ([IO.File]::ReadAllText("$inbox\hello (2).txt") -eq 'LANSend QA') 'Silent content mismatch'
 [void][ReceiptUI]::SendMessage($toggle,245,0,0)
 Run-Engine @('send',('"{0}"' -f $source))
 Start-Sleep -Milliseconds 500
 Assert-That ([ReceiptUI]::IsWindowVisible((Receipt))) 'Re-enabled popup missing'
 Write-Output 'PASS Windows HTTPS save, duplicate names, merged native window, both actions and persisted mode switch'
}finally{
 Close-TestProcess $ui
 Close-TestProcess $rx
}
