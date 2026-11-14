package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.security.PinnedTlsConnector
import org.json.JSONObject
import java.io.IOException
import javax.net.ssl.SSLSocket

private const val CLIENT_VERSION = "3.6.0"
private const val PROTOCOL_VERSION = 3

data class OpenResponse(
    val ok: Boolean,
    val code: String,
    val message: String,
    val businessEntries: List<String> = emptyList()
) {
    companion object {
        fun fromJson(json: JSONObject): OpenResponse {
            val entries = json.optJSONArray("business_entries")
            return OpenResponse(
                ok = json.optBoolean("ok", false),
                code = json.optString("code", ""),
                message = json.optString("message", ""),
                businessEntries = if (entries == null) emptyList() else List(entries.length()) { entries.optString(it) }
            )
        }
    }
}

class ProtocolException(val response: OpenResponse) : IOException(
    listOf(response.code, response.message).filter { it.isNotBlank() }.joinToString(": ")
)

class ProtocolClient(
    private val profile: MobileProfile,
    certBytes: ByteArray
) : TunnelProtocol {
    private val connector = PinnedTlsConnector(certBytes)
    @Volatile private var pool: Ipv6EndpointPool? = null
    @Volatile private var loggedIn = false

    override fun health(): OpenResponse {
        if (!loggedIn) synchronized(this) {
            if (!loggedIn) {
                val login = requestAndClose("login", null, null)
                pool = Ipv6EndpointPool(login.businessEntries, profile.serverEndpoint())
                    .takeIf { it.hasCandidates }
                loggedIn = true
                return login
            }
        }
        return requestAndClose("health", null, pool)
    }

    override fun forwardCheck(forward: ForwardConfig): OpenResponse = requestAndClose("forward_check", forward, pool)

    override fun open(forward: ForwardConfig): SSLSocket {
        val activePool = pool
        val socket = connector.connect(profile, activePool)
        try {
            LsylProtocol.writeJson(socket.outputStream, requestJson("open", forward))
            val response = OpenResponse.fromJson(LsylProtocol.readJson(socket.inputStream))
            if (!response.ok) throw ProtocolException(response)
            socket.soTimeout = 0
            connector.release(socket)
            return socket
        } catch (err: Exception) {
            connector.release(socket)
            try {
                socket.close()
            } catch (_: Exception) {
            }
            throw err
        }
    }

    override fun cancelPending() {
        connector.cancelPending()
    }

    private fun requestAndClose(type: String, forward: ForwardConfig?, activePool: Ipv6EndpointPool?): OpenResponse {
        val socket = connector.connect(profile, activePool)
        try {
            socket.use {
                LsylProtocol.writeJson(socket.outputStream, requestJson(type, forward))
                val response = OpenResponse.fromJson(LsylProtocol.readJson(socket.inputStream))
                if (!response.ok) throw ProtocolException(response)
                return response
            }
        } finally {
            connector.release(socket)
        }
    }

    private fun requestJson(type: String, forward: ForwardConfig?): JSONObject = JSONObject().apply {
        put("type", type)
        put("username", profile.username)
        put("credential", profile.savedCredential.toJson())
        put("client_id", profile.clientId)
        put("client_version", CLIENT_VERSION)
        put("protocol_version", PROTOCOL_VERSION)
        if (forward != null) {
            put("forward_name", forward.displayName())
            put("direction", forward.direction)
            put("listen_addr", forward.listenAddr)
            put("target", forward.serverTarget)
        } else {
            put("target", "")
        }
    }
}
