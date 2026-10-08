package com.spsatech.app.update

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.Settings
import androidx.core.content.FileProvider
import com.spsatech.app.BuildConfig
import com.spsatech.app.data.ServerConfig
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.security.MessageDigest

@Serializable
data class VersionInfo(
    val versionName: String,
    val versionCode: Int,
    val minVersionCode: Int = 0,
    val sizeBytes: Long = 0,
    val sha256: String,
    val url: String,
) {
    val isNewer: Boolean get() = versionCode > BuildConfig.VERSION_CODE
    /** Текущая версия больше не поддерживается — работать без обновления нельзя. */
    val isRequired: Boolean get() = BuildConfig.VERSION_CODE < minVersionCode
}

/**
 * Обновление приложения в обход магазина: версия и APK берутся
 * со страницы /app нашего сервера (публикуются deploy/publish-apk.sh).
 */
class Updater(
    private val context: Context,
    private val http: OkHttpClient,
    private val server: ServerConfig,
    private val json: Json,
) {
    private val dir get() = File(context.cacheDir, "updates")

    suspend fun check(): VersionInfo? = withContext(Dispatchers.IO) {
        val request = Request.Builder().url(server.current.value.url + "app/version.json").build()
        http.newCall(request).execute().use { resp ->
            if (!resp.isSuccessful) return@withContext null
            json.decodeFromString<VersionInfo>(resp.body!!.string())
        }
    }

    /**
     * Скачивает APK, проверяя SHA-256 из version.json.
     * [onProgress] получает долю от 0 до 1.
     */
    suspend fun download(info: VersionInfo, onProgress: (Float) -> Unit): File = withContext(Dispatchers.IO) {
        dir.mkdirs()
        dir.listFiles()?.forEach { it.delete() } // старые загрузки
        val target = File(dir, "helpdesk-${info.versionCode}.apk")
        val url = server.current.value.url.trimEnd('/') + info.url
        http.newCall(Request.Builder().url(url).build()).execute().use { resp ->
            if (!resp.isSuccessful) error("Сервер ответил ${resp.code}")
            val body = resp.body!!
            val total = body.contentLength().takeIf { it > 0 } ?: info.sizeBytes
            val digest = MessageDigest.getInstance("SHA-256")
            var done = 0L
            body.byteStream().use { input ->
                target.outputStream().use { out ->
                    val buf = ByteArray(64 * 1024)
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        out.write(buf, 0, n)
                        digest.update(buf, 0, n)
                        done += n
                        if (total > 0) onProgress(done.toFloat() / total)
                    }
                }
            }
            val sha = digest.digest().joinToString("") { "%02x".format(it) }
            if (!sha.equals(info.sha256, ignoreCase = true)) {
                target.delete()
                error("Файл повреждён при загрузке, попробуйте ещё раз")
            }
        }
        target
    }

    /** Разрешил ли пользователь этому приложению устанавливать обновления. */
    fun canInstall(): Boolean = context.packageManager.canRequestPackageInstalls()

    fun installPermissionIntent(): Intent =
        Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))

    /** Системное окно «Обновить приложение?». Android сам проверит, что подпись та же. */
    fun installIntent(apk: File): Intent {
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", apk)
        return Intent(Intent.ACTION_VIEW)
            .setDataAndType(uri, "application/vnd.android.package-archive")
            .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
    }
}
