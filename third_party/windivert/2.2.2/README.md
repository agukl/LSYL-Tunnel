# WinDivert 2.2.2

LSYL Tunnel uses the 64-bit WinDivert runtime for endpoint-specific virtual forwarding on the standard Windows client.

- Upstream: https://github.com/basil00/WinDivert
- Release: `v2.2.2`
- License: LGPL-3.0-or-later or GPL-2.0
- Supported LSYL target: 64-bit Windows 10, Windows 11, and Windows Server

The client installer ships `WinDivert.dll`, the upstream-signed `WinDivert64.sys`, the license, this notice, and the exact upstream source archive. Win7 Lite and Android do not package or load WinDivert.

## SHA-256

```text
14A0CB5214D536E4FDAE6AA3F5696F981EEDA106CD026E9794BBA489EE79D628  LICENSE
65EC79C9E6AFA99F648A3F4D1F6DB794640B40D0B65BD438770EA503EE14ECB7  source/WinDivert-2.2.2-source.zip
C1E060EE19444A259B2162F8AF0F3FE8C4428A1C6F694DCE20DE194AC8D7D9A2  x64/WinDivert.dll
8DA085332782708D8767BCACE5327A6EC7283C17CFB85E40B03CD2323A90DDC2  x64/WinDivert64.sys
```
