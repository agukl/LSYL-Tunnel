package com.lsyl.tunnel.mobile.profile

import android.content.Context
import java.io.File

private const val ACTIVE_DIR = "active-profile"

class ProfileStore(private val context: Context) {
    private val dir: File = File(context.filesDir, ACTIVE_DIR)
    private val profileFile: File = File(dir, "profile.json")
    private val certFile: File = File(dir, "server.crt")

    fun save(imported: ImportedProfile) {
        if (!dir.exists()) dir.mkdirs()
        profileFile.writeText(imported.rawProfileJson.toString(2), Charsets.UTF_8)
        certFile.writeBytes(imported.serverCertBytes)
    }

    fun load(): LoadedProfile? {
        if (!profileFile.isFile || !certFile.isFile) return null
        val json = org.json.JSONObject(profileFile.readText(Charsets.UTF_8))
        val certBytes = certFile.readBytes()
        val profile = MobileProfile.fromJson(json)
        return LoadedProfile(profile, certBytes)
    }

    fun delete() {
        certFile.delete()
        profileFile.delete()
        dir.delete()
    }
}

data class LoadedProfile(
    val profile: MobileProfile,
    val serverCertBytes: ByteArray
)
