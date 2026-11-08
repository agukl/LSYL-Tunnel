package com.lsyl.tunnel.mobile.profile

import android.content.Context
import android.net.Uri
import com.lsyl.tunnel.mobile.security.CertificatePins
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.time.OffsetDateTime
import java.util.zip.ZipInputStream

private const val PROFILE_JSON = "profile.json"
private const val SERVER_CERT = "server.crt"
private const val MAX_PROFILE_ARCHIVE_BYTES = 16 * 1024 * 1024
private const val MAX_PROFILE_ENTRY_BYTES = 8 * 1024 * 1024
private const val MAX_PROFILE_ENTRIES = 32

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
            return importFromZip(input.readLimitedBytes(MAX_PROFILE_ARCHIVE_BYTES, "配置包"))
        }
    }

    fun importFromZip(zipBytes: ByteArray): ImportedProfile {
        if (zipBytes.size > MAX_PROFILE_ARCHIVE_BYTES) throw ProfileImportException("配置包超过大小限制")
        var profileBytes: ByteArray? = null
        var certBytes: ByteArray? = null
        var entryCount = 0
        ZipInputStream(zipBytes.inputStream()).use { zip ->
            while (true) {
                val entry = zip.nextEntry ?: break
                if (++entryCount > MAX_PROFILE_ENTRIES) throw ProfileImportException("配置包条目过多")
                if (entry.isDirectory) continue
                val name = entry.name.substringAfterLast('/').substringAfterLast('\\')
                when (name) {
                    PROFILE_JSON -> {
                        if (profileBytes != null) throw ProfileImportException("配置包包含重复的 $name")
                        profileBytes = zip.readLimitedBytes(MAX_PROFILE_ENTRY_BYTES, name)
                    }
                    SERVER_CERT -> {
                        if (certBytes != null) throw ProfileImportException("配置包包含重复的 $name")
                        certBytes = zip.readLimitedBytes(MAX_PROFILE_ENTRY_BYTES, name)
                    }
                    else -> throw ProfileImportException("配置包包含未知文件: $name")
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

    private fun InputStream.readLimitedBytes(limit: Int, label: String): ByteArray {
        val out = ByteArrayOutputStream()
        val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
        while (true) {
            val n = read(buffer)
            if (n < 0) break
            if (n == 0) continue
            if (out.size() + n > limit) throw ProfileImportException("$label 超过大小限制")
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
        MobileForwardValidator.validate(profile.forwards)
        CertificatePins.parseCertificate(certBytes)
    }
}
