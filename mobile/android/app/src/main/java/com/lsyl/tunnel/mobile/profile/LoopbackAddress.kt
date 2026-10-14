package com.lsyl.tunnel.mobile.profile

import java.net.InetAddress

internal fun requireCanonicalLoopbackHost(host: String, field: String) {
    val value = host.trim()
    if (value == "127.0.0.1") return
    val name = value.lowercase().removeSuffix(".")
    val literal = value.removeSurrounding("[", "]").substringBefore('%')
    val octets = literal.split('.')
    val ipv4Loopback = octets.size == 4 && octets.first().toIntOrNull() == 127 &&
        octets.all { it.toIntOrNull()?.let { octet -> octet in 0..255 } == true }
    // Only numeric IPv6 literals reach InetAddress; validation never resolves DNS.
    val ipv6Loopback = ':' in literal && literal.all { it in "0123456789abcdefABCDEF:." } &&
        runCatching { InetAddress.getByName(literal).isLoopbackAddress }.getOrDefault(false)
    if (name == "localhost" || name.endsWith(".localhost") || ipv4Loopback || ipv6Loopback) {
        throw ProfileImportException("$field 不再支持回环地址 $host，请使用 127.0.0.1")
    }
}
