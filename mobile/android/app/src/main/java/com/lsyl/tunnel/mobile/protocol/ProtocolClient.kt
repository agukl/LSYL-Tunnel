package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.security.PinnedTlsConnector
import org.json.JSONObject
import java.io.IOException
import javax.net.ssl.SSLSocket

private const val CLIENT_VERSION = "2.1.0"
private const val PROTOCOL_VERSION = 2

data class OpenResponse(
    val ok: Boolean,
    val code: String,
    val message: String
) {
    companion object {
        fun fromJson(json: JSONObject): OpenResponse = OpenResponse(
            ok = json.optBoolean("ok", false),
            code = json.optString("code", ""),
            message = json.optString("message", "")
        )
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

    override fun health(): OpenResponse = requestAndClose("health", null)

    override fun forwardCheck(forward: ForwardConfig): OpenResponse = requestAndClose("forward_check", forward)

    override fun open(forward: ForwardConfig): SSLSocket {
        val socket = connector.connect(profile)
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

    private fun requestAndClose(type: String, forward: ForwardConfig?): OpenResponse {
        val socket = connector.connect(profile)
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
