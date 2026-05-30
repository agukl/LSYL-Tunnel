package com.lsyl.tunnel.mobile.protocol

import com.lsyl.tunnel.mobile.profile.ForwardConfig
import com.lsyl.tunnel.mobile.profile.MobileProfile
import com.lsyl.tunnel.mobile.security.PinnedTlsConnector
import org.json.JSONObject
import java.io.IOException
import javax.net.ssl.SSLSocket

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
) {
    private val connector = PinnedTlsConnector(certBytes)

    fun health(): OpenResponse = requestAndClose("health", null)

    fun forwardCheck(forward: ForwardConfig): OpenResponse = requestAndClose("forward_check", forward)

    fun open(forward: ForwardConfig): SSLSocket {
        val socket = connector.connect(profile)
        try {
            LsylProtocol.writeJson(socket.outputStream, requestJson("open", forward))
            val response = OpenResponse.fromJson(LsylProtocol.readJson(socket.inputStream))
            if (!response.ok) throw ProtocolException(response)
            socket.soTimeout = 0
            return socket
        } catch (err: Exception) {
            try {
                socket.close()
            } catch (_: Exception) {
            }
            throw err
        }
    }

    private fun requestAndClose(type: String, forward: ForwardConfig?): OpenResponse {
        connector.connect(profile).use { socket ->
            LsylProtocol.writeJson(socket.outputStream, requestJson(type, forward))
            val response = OpenResponse.fromJson(LsylProtocol.readJson(socket.inputStream))
            if (!response.ok) throw ProtocolException(response)
            return response
        }
    }

    private fun requestJson(type: String, forward: ForwardConfig?): JSONObject = JSONObject().apply {
        put("type", type)
        put("username", profile.username)
        put("credential", profile.savedCredential.toJson())
        put("client_id", profile.clientId)
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
