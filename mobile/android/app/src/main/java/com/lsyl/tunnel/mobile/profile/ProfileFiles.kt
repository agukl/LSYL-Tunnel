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

    fun save(imported: ImportedProfile) {
        ensureDirectory()
        atomicWrite(certFile, imported.serverCertBytes)
        atomicWrite(profileFile, imported.rawProfileJson.toString(2).toByteArray(Charsets.UTF_8))
    }

    fun load(): LoadedProfile? {
        if (!profileFile.isFile || !certFile.isFile) return null
        val json = JSONObject(profileFile.readText(Charsets.UTF_8))
        val certBytes = certFile.readBytes()
        return LoadedProfile(MobileProfile.fromJson(json), certBytes)
    }

    fun updateForwards(forwards: List<ForwardConfig>) {
        MobileForwardValidator.validate(forwards)
        if (!profileFile.isFile || !certFile.isFile) throw ProfileImportException("未导入连接配置")
        val json = JSONObject(profileFile.readText(Charsets.UTF_8))
        json.put("forwards", JSONArray().also { array ->
            forwards.forEach { array.put(it.toJson()) }
        })
        atomicWrite(profileFile, json.toString(2).toByteArray(Charsets.UTF_8))
    }

    fun delete() {
        File(dir, "$PROFILE_JSON_FILE.tmp").delete()
        File(dir, "$SERVER_CERT_FILE.tmp").delete()
        certFile.delete()
        profileFile.delete()
        dir.delete()
    }

    private fun atomicWrite(target: File, bytes: ByteArray) {
        ensureDirectory()
        val temporary = File(dir, "${target.name}.tmp")
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

    private fun ensureDirectory() {
        if (!dir.exists() && !dir.mkdirs()) throw ProfileImportException("无法创建配置目录")
        if (!dir.isDirectory) throw ProfileImportException("配置目录不可用")
    }

    private companion object {
        const val PROFILE_JSON_FILE = "profile.json"
        const val SERVER_CERT_FILE = "server.crt"
    }
}
