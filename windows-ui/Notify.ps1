param([Parameter(Mandatory=$true)][ValidatePattern('^[A-Za-z0-9_-]+$')][string]$Token)
$ErrorActionPreference = 'Stop'
$encoded = $Token.Replace('-','+').Replace('_','/')
while (($encoded.Length % 4) -ne 0) { $encoded += '=' }
$name = [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($encoded))
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] > $null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml('<toast activationType="protocol"><visual><binding template="ToastGeneric"><text>LANSend 已收到文件</text><text/></binding></visual></toast>')
$xml.DocumentElement.SetAttribute('launch', "lansend-reveal://file/$Token")
$xml.GetElementsByTagName('text').Item(1).InnerText = $name
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('LANSend.Receipts').Show($toast)
