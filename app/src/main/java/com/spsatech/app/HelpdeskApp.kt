package com.spsatech.app

import android.app.Application
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.SingletonImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import com.spsatech.app.data.HelpdeskApi
import com.spsatech.app.data.HelpdeskRepository
import com.spsatech.app.data.ServerConfig
import com.spsatech.app.data.SessionState
import com.spsatech.app.data.SessionStore
import com.spsatech.app.push.Notifications
import com.spsatech.app.push.PushRegistrar
import com.spsatech.app.update.Updater
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.logging.HttpLoggingInterceptor
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.util.concurrent.TimeUnit

/** Зависимости приложения, создаются один раз. */
class HelpdeskApp : Application(), SingletonImageLoader.Factory {
    lateinit var repository: HelpdeskRepository
        private set
    lateinit var session: SessionStore
        private set
    lateinit var server: ServerConfig
        private set
    lateinit var pushRegistrar: PushRegistrar
        private set
    lateinit var updater: Updater
        private set

    /** id заявок, по которым пришло push-уведомление: открытые экраны обновляются сами. */
    val ticketUpdates = MutableSharedFlow<Long>(extraBufferCapacity = 16)
    private lateinit var okHttp: OkHttpClient

    override fun onCreate() {
        super.onCreate()
        val scope = CoroutineScope(SupervisorJob())
        val json = Json { ignoreUnknownKeys = true }
        session = SessionStore(this, json, scope)
        server = ServerConfig(this)

        val prodUrl = BuildConfig.PROD_API_URL.toHttpUrl()
        okHttp = OkHttpClient.Builder()
            // Retrofit собран с адресом боевого сервера; если выбран другой — подменяем хост.
            .addInterceptor { chain ->
                val target = server.current.value.url.toHttpUrl()
                val url = chain.request().url
                val request = if (url.host == prodUrl.host && target != prodUrl) {
                    chain.request().newBuilder()
                        .url(url.newBuilder().scheme(target.scheme).host(target.host).port(target.port).build())
                        .build()
                } else {
                    chain.request()
                }
                chain.proceed(request)
            }
            .addInterceptor { chain ->
                val token = session.token
                val request = if (token != null) {
                    chain.request().newBuilder().header("Authorization", "Bearer $token").build()
                } else {
                    chain.request()
                }
                val response = chain.proceed(request)
                // Токен отозван или истёк — выходим, приложение покажет экран входа.
                if (response.code == 401 && token != null && session.state.value is SessionState.LoggedIn) {
                    scope.launch { session.clear() }
                }
                response
            }
            .apply {
                if (BuildConfig.DEBUG) {
                    addInterceptor(HttpLoggingInterceptor().setLevel(HttpLoggingInterceptor.Level.BASIC))
                }
            }
            .connectTimeout(15, TimeUnit.SECONDS)
            .writeTimeout(60, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .build()

        val api = Retrofit.Builder()
            .baseUrl(prodUrl)
            .client(okHttp)
            .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
            .build()
            .create(HelpdeskApi::class.java)

        repository = HelpdeskRepository(api, session, json)

        // Для загрузки обновлений — без токена и с длинным таймаутом чтения.
        updater = Updater(this, okHttp.newBuilder().readTimeout(2, TimeUnit.MINUTES).build(), server, json)

        Notifications.createChannels(this)
        pushRegistrar = PushRegistrar(repository, session, scope).also { it.start() }
    }

    // Фото заявок отдаются только с токеном, поэтому Coil ходит через тот же OkHttpClient.
    override fun newImageLoader(context: PlatformContext): ImageLoader =
        ImageLoader.Builder(context)
            .components { add(OkHttpNetworkFetcherFactory(callFactory = { okHttp })) }
            .build()
}
