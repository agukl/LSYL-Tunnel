package com.lsyl.tunnel.mobile.profile

import java.net.InetAddress

// Match the server's literal-IP policy without resolving names on the phone.
internal fun isForwardTargetHost(host: String): Boolean {
    if (host == "127.0.0.1") return true
    val bytes = if (':' in host) {
        if (host.any { it !in "0123456789abcdefABCDEF:." }) return false
        runCatching { InetAddress.getByName(host).address }.getOrNull() ?: return false
    } else {
        val octets = host.split('.')
        if (octets.size != 4) return false
        if (octets.any { it.isEmpty() || it.any { c -> c !in '0'..'9' } ||
                (it.length > 1 && it.startsWith('0')) || (it.toIntOrNull() ?: -1) !in 0..255 }) return false
        octets.map { it.toInt().toByte() }.toByteArray()
    }
    val first = bytes[0].toInt() and 0xff
    if (bytes.size == 16) return first and 0xfe == 0xfc
    if (bytes.size != 4) return false
    val second = bytes[1].toInt() and 0xff
    return first == 10 || (first == 172 && second in 16..31) || (first == 192 && second == 168)
}
