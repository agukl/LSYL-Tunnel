package com.lsyl.tunnel.mobile.profile

import android.content.Context
import android.net.Uri
import com.lsyl.tunnel.mobile.security.CertificatePins
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.time.OffsetDateTime
import java.util.zip.ZipInputStream

private const val PROFILE_JSON = "profile.json"
private const val SERVER_CERT = "server.crt"

data class ImportedProfile(
    val profile: MobileProfile,
    val rawProfileJson: JSONObject,
    val serverCertBytes: ByteArray,
    val serverCertSha256: String
)

class ProfileImportException(message: String) : IllegalArgumentException(message)

object ProfileImporter {
    fun importFromUri(context: Context, uri: Uri): ImportedProfile {
        context.contentResolver.openInputStream(uri).use { input ->
            requireNotNull(input) { "cannot open profile file" }
            return importFromZip(input.readBytes())
        }
    }

    fun importFromZip(zipBytes: ByteArray): ImportedProfile {
        var profileBytes: ByteArray? = null
        var certBytes: ByteArray? = null
        ZipInputStream(zipBytes.inputStream()).use { zip ->
            while (true) {
                val entry = zip.nextEntry ?: break
                if (entry.isDirectory) continue
                val name = entry.name.substringAfterLast('/').substringAfterLast('\\')
                val data = zip.readCurrentEntry()
                when (name) {
                    PROFILE_JSON -> profileBytes = data
                    SERVER_CERT -> certBytes = data
                }
            }
        }
        val rawProfile = profileBytes ?: throw ProfileImportException("缺少 profile.json")
        val rawCert = certBytes ?: throw ProfileImportException("缺少 server.crt")
        val json = JSONObject(rawProfile.toString(Charsets.UTF_8))
        val profile = MobileProfile.fromJson(json)
        validateRawProfile(json)
        validateProfile(profile, rawCert)
        val cert = CertificatePins.parseCertificate(rawCert)
        return ImportedProfile(
            profile = profile,
            rawProfileJson = profile.toJson(),
            serverCertBytes = rawCert,
            serverCertSha256 = CertificatePins.sha256Hex(cert)
        )
    }

    private fun ZipInputStream.readCurrentEntry(): ByteArray {
        val out = ByteArrayOutputStream()
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        while (true) {
            val n = read(buffer)
            if (n <= 0) break
            out.write(buffer, 0, n)
        }
        return out.toByteArray()
    }

    private fun validateRawProfile(json: JSONObject) {
        listOf("password", "password_env", "password_file").forEach { key ->
            if (json.has(key)) throw ProfileImportException("移动端 Profile 不允许包含 $key")
        }
    }

    private fun validateProfile(profile: MobileProfile, certBytes: ByteArray) {
        if (profile.version != 1) throw ProfileImportException("不支持的 Profile 版本")
        if (profile.serverAddr.isBlank()) throw ProfileImportException("缺少服务端地址")
        profile.serverEndpoint()
        if (profile.username.isBlank()) throw ProfileImportException("缺少用户名")
        if (profile.savedCredential.type != "server_sealed") throw ProfileImportException("凭证类型不受支持")
        if (profile.savedCredential.keyId.isBlank()) throw ProfileImportException("缺少凭证 key_id")
        if (profile.savedCredential.ciphertext.isBlank()) throw ProfileImportException("缺少登录凭据")
        if (!profile.savedCredential.expiresAtDateTime().isAfter(OffsetDateTime.now())) {
            throw ProfileImportException("登录凭据已过期")
        }
        if (profile.tls.insecureSkipVerify) throw ProfileImportException("移动端不允许跳过证书校验")
        if (profile.tls.minVersion != "1.3") throw ProfileImportException("移动端要求 TLS 1.3")
        CertificatePins.parseCertificate(certBytes)
        if (profile.forwards.isEmpty()) throw ProfileImportException("至少需要一个正向端口")
        val names = mutableSetOf<String>()
        val listens = mutableSetOf<String>()
        profile.forwards.forEach { forward ->
            if (forward.direction != DIRECTION_CLIENT_TO_SERVER) {
                throw ProfileImportException("移动端仅支持正向代理")
            }
            val local = forward.localEndpoint()
            if (local.host != "127.0.0.1") throw ProfileImportException("本地监听只能使用 127.0.0.1")
            if (local.port < 1024) throw ProfileImportException("本地端口必须大于等于 1024")
            if (forward.serverTarget.isBlank()) throw ProfileImportException("缺少服务端目标端口")
            HostPort.parse(forward.serverTarget)
            val name = forward.displayName()
            if (!names.add(name)) throw ProfileImportException("端口名称重复: $name")
            if (!listens.add(forward.listenAddr)) throw ProfileImportException("本地监听端口重复: ${forward.listenAddr}")
        }
    }
}
