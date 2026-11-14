package com.lsyl.tunnel.mobile.profile

import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.FileOutputStream
import java.nio.file.AtomicMoveNotSupportedException
import java.nio.file.Files
import java.nio.file.StandardCopyOption

internal class ProfileFiles(private val dir: File) {
    private val profileFile = File(dir, PROFILE_JSON_FILE)
    private val certFile = File(dir, SERVER_CERT_FILE)
    private val aliasFile = File(dir, ALIAS_FILE)

    fun save(imported: ImportedProfile) {
        ensureDirectory()
        atomicWriteProfileFile(certFile, imported.serverCertBytes)
        atomicWriteProfileFile(profileFile, imported.rawProfileJson.toString(2).toByteArray(Charsets.UTF_8))
    }

    fun load(): LoadedProfile? {
        if (!profileFile.isFile || !certFile.isFile) return null
        val json = JSONObject(profileFile.readText(Charsets.UTF_8))
        val certBytes = certFile.readBytes()
        val profile = MobileProfile.fromJson(json)
        profile.serverEndpoint()
        MobileForwardValidator.validate(profile.forwards)
        return LoadedProfile(profile, certBytes)
    }

    fun updateForwards(forwards: List<ForwardConfig>) {
        MobileForwardValidator.validate(forwards)
        if (!profileFile.isFile || !certFile.isFile) throw ProfileImportException("未导入连接配置")
        val json = JSONObject(profileFile.readText(Charsets.UTF_8))
        json.put("forwards", JSONArray().also { array ->
            forwards.forEach { array.put(it.toJson()) }
        })
        atomicWriteProfileFile(profileFile, json.toString(2).toByteArray(Charsets.UTF_8))
    }

    fun saveAlias(alias: String) {
        val value = alias.trim().replace(Regex("\\s+"), " ")
        if (value.length > 40) throw ProfileImportException("配置别名不能超过 40 个字符")
        ensureDirectory()
        atomicWriteProfileFile(aliasFile, value.toByteArray(Charsets.UTF_8))
    }

    fun readAlias(): String = if (aliasFile.isFile) aliasFile.readText(Charsets.UTF_8).trim() else ""

    fun delete() {
        File(dir, "$PROFILE_JSON_FILE.tmp").delete()
        File(dir, "$SERVER_CERT_FILE.tmp").delete()
        File(dir, "$ALIAS_FILE.tmp").delete()
        aliasFile.delete()
        certFile.delete()
        profileFile.delete()
        dir.delete()
    }

    private fun ensureDirectory() {
        if (!dir.exists() && !dir.mkdirs()) throw ProfileImportException("无法创建配置目录")
        if (!dir.isDirectory) throw ProfileImportException("配置目录不可用")
    }

    private companion object {
        const val PROFILE_JSON_FILE = "profile.json"
        const val SERVER_CERT_FILE = "server.crt"
        const val ALIAS_FILE = "alias.txt"
    }
}

internal fun atomicWriteProfileFile(target: File, bytes: ByteArray) {
    val temporary = File(target.parentFile, "${target.name}.tmp")
    try {
        FileOutputStream(temporary).use { output ->
            output.write(bytes)
            output.flush()
            output.fd.sync()
        }
        try {
            Files.move(
                temporary.toPath(),
                target.toPath(),
                StandardCopyOption.ATOMIC_MOVE,
                StandardCopyOption.REPLACE_EXISTING
            )
        } catch (_: AtomicMoveNotSupportedException) {
            Files.move(temporary.toPath(), target.toPath(), StandardCopyOption.REPLACE_EXISTING)
        }
    } finally {
        temporary.delete()
    }
}
