package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.HostPort
import java.net.Inet6Address
import java.net.InetAddress
import java.util.concurrent.TimeUnit

internal class Ipv6EndpointPool(
    advertised: List<String>,
    private val bootstrap: HostPort,
    private val nowMillis: () -> Long = System::currentTimeMillis
) {
    private data class Candidate(val endpoint: String, val address: HostPort)
    private data class Quality(
        var attempts: Int = 0,
        var successes: Int = 0,
        var totalDialMs: Long = 0
    )

    private val candidates = advertised.asSequence().mapNotNull(::parseCandidate)
        .distinctBy { it.endpoint }.take(MAX_CANDIDATES).toList()
    private val quality = candidates.map { it.endpoint }
        .associateWith { Quality() }.toMutableMap()
    private val failedUntil = mutableMapOf<String, Long>()
    private var qualityStarted = nowMillis()
    private var next = 0

    val hasCandidates: Boolean get() = candidates.isNotEmpty()

    fun <T> dial(timeoutMillis: Int, connect: (HostPort, Int) -> T): T {
        val ordered = orderedCandidates()
        var attempted = 0
        for (candidate in ordered) {
            if (nowMillis() < synchronized(this) { failedUntil[candidate.endpoint] ?: 0L }) continue
            attempted++
            val started = System.nanoTime()
            try {
                return connect(candidate.address, minOf(timeoutMillis, CANDIDATE_TIMEOUT_MS)).also {
                    record(candidate.endpoint, started, null)
                }
            } catch (failure: Exception) {
                record(candidate.endpoint, started, failure)
                synchronized(this) { failedUntil[candidate.endpoint] = nowMillis() + RETRY_DELAY_MS }
                if (attempted >= MAX_CANDIDATE_ATTEMPTS) break
            }
        }
        return connect(bootstrap, timeoutMillis)
    }

    @Synchronized private fun orderedCandidates(): List<Candidate> {
        rollWindow()
        if (candidates.isEmpty()) return emptyList()
        val start = next++ % candidates.size
        val rotated = List(candidates.size) { candidates[(start + it) % candidates.size] }
        return rotated.sortedWith { a, b ->
            val left = quality.getValue(a.endpoint)
            val right = quality.getValue(b.endpoint)
            when {
                left.attempts == 0 && right.attempts != 0 -> -1
                left.attempts != 0 && right.attempts == 0 -> 1
                left.successes.toLong() * right.attempts != right.successes.toLong() * left.attempts ->
                    (right.successes.toLong() * left.attempts).compareTo(left.successes.toLong() * right.attempts)
                left.successes > 0 && right.successes > 0 ->
                    (left.totalDialMs / left.successes).compareTo(right.totalDialMs / right.successes)
                else -> 0
            }
        }
    }

    @Synchronized private fun record(endpoint: String, started: Long, failure: Exception?) {
        rollWindow()
        val item = quality.getValue(endpoint)
        if (item.attempts >= MAX_ATTEMPTS) return
        item.attempts++
        if (failure == null) {
            item.successes++
            item.totalDialMs += TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started).coerceIn(0, 60000)
        }
    }

    private fun rollWindow() {
        val now = nowMillis()
        if (now - qualityStarted < QUALITY_WINDOW_MS) return
        quality.values.forEach {
            it.attempts = 0
            it.successes = 0
            it.totalDialMs = 0
        }
        qualityStarted = now
    }

    private fun parseCandidate(value: String): Candidate? {
        val endpoint = value.trim()
        val match = IPV6_ENDPOINT.matchEntire(endpoint) ?: return null
        val host = match.groupValues[1]
        val port = match.groupValues[2].toIntOrNull() ?: return null
        if (port != bootstrap.port || '%' in host) return null
        val address = runCatching { InetAddress.getByName(host) }.getOrNull() as? Inet6Address ?: return null
        val bytes = address.address
        if (address.isAnyLocalAddress || address.isLoopbackAddress || address.isLinkLocalAddress ||
            address.isSiteLocalAddress || address.isMulticastAddress || (bytes[0].toInt() and 0xfe) == 0xfc
        ) return null
        return Candidate(endpoint, HostPort(host, port))
    }

    private companion object {
        val IPV6_ENDPOINT = Regex("^\\[([^]]+)]:(\\d+)$")
        const val MAX_CANDIDATES = 64
        const val MAX_CANDIDATE_ATTEMPTS = 2
        const val MAX_ATTEMPTS = 1_000_000
        const val CANDIDATE_TIMEOUT_MS = 1200
        const val RETRY_DELAY_MS = 30_000L
        const val QUALITY_WINDOW_MS = 60_000L
    }
}
