package com.spsatech.app.data

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.serialization.json.Json

private val Context.dataStore by preferencesDataStore(name = "session")

data class Session(val token: String, val user: User)

/** Состояние входа: пока DataStore не прочитан — Loading. */
sealed interface SessionState {
    data object Loading : SessionState
    data object LoggedOut : SessionState
    data class LoggedIn(val session: Session) : SessionState
}

class SessionStore(private val context: Context, private val json: Json, scope: CoroutineScope) {
    private val tokenKey = stringPreferencesKey("token")
    private val userKey = stringPreferencesKey("user")

    val state: StateFlow<SessionState> = context.dataStore.data
        .map { prefs ->
            val token = prefs[tokenKey]
            val user = prefs[userKey]?.let { runCatching { json.decodeFromString<User>(it) }.getOrNull() }
            if (token != null && user != null) SessionState.LoggedIn(Session(token, user)) else SessionState.LoggedOut
        }
        .stateIn(scope, SharingStarted.Eagerly, SessionState.Loading)

    /** Текущий токен для подстановки в запросы. */
    val token: String?
        get() = (state.value as? SessionState.LoggedIn)?.session?.token

    suspend fun save(token: String, user: User) {
        context.dataStore.edit {
            it[tokenKey] = token
            it[userKey] = json.encodeToString(user)
        }
    }

    suspend fun clear() {
        context.dataStore.edit { it.clear() }
    }
}
