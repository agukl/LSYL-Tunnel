package com.lsyl.tunnel.mobile.profile

import org.json.JSONArray
import org.json.JSONObject
import java.time.OffsetDateTime

const val DIRECTION_CLIENT_TO_SERVER = "client_to_server"

/** MobileProfile is intentionally narrower than the desktop client config. */
data class MobileProfile(
    val version: Int,
    val profileId: String?,
    val serverAddr: String,
    val username: String,
    val clientId: String,
    val savedCredential: SavedCredential,
    val tls: TlsConfig,
    val connection: ConnectionConfig,
    val forwards: List<ForwardConfig>
) {
    fun serverEndpoint(): HostPort = HostPort.parse(serverAddr)

    fun toJson(): JSONObject = JSONObject().apply {
        put("version", version)
        profileId?.let { put("profile_id", it) }
        put("server_addr", serverAddr)
        put("username", username)
        put("client_id", clientId)
        put("saved_credential", savedCredential.toJson())
        put("tls", tls.toJson())
        put("connection", connection.toJson())
        put("forwards", JSONArray().also { arr -> forwards.forEach { arr.put(it.toJson()) } })
    }

    companion object {
        fun fromJson(json: JSONObject): MobileProfile {
            val credential = json.getJSONObject("saved_credential")
            val tls = json.optJSONObject("tls") ?: JSONObject()
            val connection = json.optJSONObject("connection") ?: JSONObject()
            val forwardsJson = json.getJSONArray("forwards")
            val forwards = buildList {
                for (i in 0 until forwardsJson.length()) {
                    add(ForwardConfig.fromJson(forwardsJson.getJSONObject(i)))
                }
            }
            return MobileProfile(
                version = json.optInt("version", 1),
                profileId = json.optStringOrNull("profile_id"),
                serverAddr = json.getString("server_addr").trim(),
                username = json.getString("username").trim(),
                clientId = json.optString("client_id", "").trim(),
                savedCredential = SavedCredential.fromJson(credential),
                tls = TlsConfig.fromJson(tls),
                connection = ConnectionConfig.fromJson(connection),
                forwards = forwards
            )
        }
    }
}

data class SavedCredential(
    val type: String,
    val keyId: String,
    val expiresAt: String,
    val ciphertext: String
) {
    fun expiresAtDateTime(): OffsetDateTime = OffsetDateTime.parse(expiresAt)

    fun toJson(): JSONObject = JSONObject().apply {
        put("type", type)
        put("key_id", keyId)
        put("expires_at", expiresAt)
        put("ciphertext", ciphertext)
    }

    companion object {
        fun fromJson(json: JSONObject): SavedCredential = SavedCredential(
            type = json.getString("type").trim(),
            keyId = json.getString("key_id").trim(),
            expiresAt = json.getString("expires_at").trim(),
            ciphertext = json.getString("ciphertext").trim()
        )
    }
}

data class TlsConfig(
    val caCertFile: String,
    val serverName: String,
    val minVersion: String,
    val insecureSkipVerify: Boolean
) {
    fun toJson(): JSONObject = JSONObject().apply {
        put("ca_cert_file", caCertFile)
        put("server_name", serverName)
        put("min_version", minVersion)
        put("insecure_skip_verify", insecureSkipVerify)
    }

    companion object {
        fun fromJson(json: JSONObject): TlsConfig = TlsConfig(
            caCertFile = json.optString("ca_cert_file", "server.crt").trim(),
            serverName = json.optString("server_name", "").trim(),
            minVersion = json.optString("min_version", "1.3").trim(),
            insecureSkipVerify = json.optBoolean("insecure_skip_verify", false)
        )
    }
}

data class ConnectionConfig(val dialTimeoutSec: Int) {
    fun timeoutMillis(): Int = (if (dialTimeoutSec > 0) dialTimeoutSec else 5) * 1000

    fun toJson(): JSONObject = JSONObject().apply {
        put("dial_timeout_sec", dialTimeoutSec)
    }

    companion object {
        fun fromJson(json: JSONObject): ConnectionConfig = ConnectionConfig(
            dialTimeoutSec = json.optInt("dial_timeout_sec", 5).takeIf { it > 0 } ?: 5
        )
    }
}

data class ForwardConfig(
    val name: String,
    val direction: String,
    val listenAddr: String,
    val serverTarget: String
) {
    fun displayName(): String = name.ifBlank { listenAddr }
    fun localEndpoint(): HostPort = HostPort.parse(listenAddr)

    fun toJson(): JSONObject = JSONObject().apply {
        put("name", name)
        put("direction", direction)
        put("listen_addr", listenAddr)
        put("server_target", serverTarget)
    }

    companion object {
        fun fromJson(json: JSONObject): ForwardConfig = ForwardConfig(
            name = json.optString("name", "").trim(),
            direction = json.optString("direction", DIRECTION_CLIENT_TO_SERVER).trim().ifBlank { DIRECTION_CLIENT_TO_SERVER },
            listenAddr = json.getString("listen_addr").trim(),
            serverTarget = json.getString("server_target").trim()
        )
    }
}

data class HostPort(val host: String, val port: Int) {
    companion object {
        fun parse(value: String): HostPort {
            val text = value.trim()
            require(text.isNotEmpty()) { "address is empty" }
            if (text.startsWith("[")) {
                val close = text.indexOf(']')
                require(close > 0 && close + 2 <= text.length && text[close + 1] == ':') { "invalid address: $value" }
                return HostPort(text.substring(1, close), parsePort(text.substring(close + 2), value))
            }
            val idx = text.lastIndexOf(':')
            require(idx > 0 && idx < text.length - 1) { "invalid address: $value" }
            return HostPort(text.substring(0, idx), parsePort(text.substring(idx + 1), value))
        }

        private fun parsePort(text: String, original: String): Int {
            val port = text.toIntOrNull()
            require(port != null && port in 1..65535) { "invalid port in address: $original" }
            return port
        }
    }
}

fun JSONObject.optStringOrNull(name: String): String? =
    if (has(name) && !isNull(name)) optString(name).trim().ifBlank { null } else null
