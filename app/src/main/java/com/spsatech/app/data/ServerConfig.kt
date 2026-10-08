package com.spsatech.app.data

import android.content.Context
import androidx.core.content.edit
import com.spsatech.app.BuildConfig
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class Server(val id: String, val name: String, val url: String)

/**
 * Адрес сервера. В release-сборке он один — боевой; в debug можно
 * переключиться на локальный сервер разработки (выбор запоминается).
 */
class ServerConfig(context: Context) {
    private val prefs = context.getSharedPreferences("server", Context.MODE_PRIVATE)

    val servers: List<Server> = buildList {
        if (BuildConfig.DEV_API_URL.isNotEmpty()) add(Server("dev", "Локальный (Mac)", BuildConfig.DEV_API_URL))
        add(Server("prod", "Боевой", BuildConfig.PROD_API_URL))
    }

    val canSwitch: Boolean get() = servers.size > 1

    private val _current = MutableStateFlow(
        servers.firstOrNull { it.id == prefs.getString("id", null) } ?: servers.first()
    )
    val current: StateFlow<Server> = _current.asStateFlow()

    fun select(server: Server) {
        prefs.edit { putString("id", server.id) }
        _current.value = server
    }

    fun attachmentUrl(id: Long) = "${current.value.url}api/attachments/$id"
}
