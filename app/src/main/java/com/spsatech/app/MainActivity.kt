package com.spsatech.app

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.platform.LocalContext
import androidx.core.content.ContextCompat
import com.spsatech.app.push.Notifications
import com.spsatech.app.update.UpdateGate
import kotlinx.coroutines.flow.MutableStateFlow
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import com.spsatech.app.data.SessionState
import com.spsatech.app.data.User
import com.spsatech.app.ui.common.FullScreenLoading
import com.spsatech.app.ui.login.LoginScreen
import com.spsatech.app.ui.newticket.NewTicketScreen
import com.spsatech.app.ui.password.ChangePasswordScreen
import com.spsatech.app.ui.theme.SpsatechTheme
import com.spsatech.app.ui.ticket.TicketDetailScreen
import com.spsatech.app.ui.tickets.TicketListScreen
import kotlinx.serialization.Serializable

class MainActivity : ComponentActivity() {
    /** Заявка, которую нужно открыть (пришли по нажатию на уведомление). */
    private val openTicket = MutableStateFlow<Long?>(null)

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        takeTicket(intent)
    }

    private fun takeTicket(intent: Intent?) {
        intent?.getLongExtra(Notifications.EXTRA_TICKET_ID, 0L)?.takeIf { it > 0 }?.let {
            openTicket.value = it
            intent.removeExtra(Notifications.EXTRA_TICKET_ID)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        if (savedInstanceState == null) takeTicket(intent)
        val session = (application as HelpdeskApp).session
        setContent {
            SpsatechTheme { UpdateGate {
                val state by session.state.collectAsStateWithLifecycle()
                when (val s = state) {
                    SessionState.Loading -> FullScreenLoading()
                    SessionState.LoggedOut -> LoginScreen()
                    is SessionState.LoggedIn ->
                        if (s.session.user.mustChangePassword) {
                            ChangePasswordScreen(forced = true, onDone = {})
                        } else {
                            NotificationPermission()
                            MainNavigation(s.session.user, openTicket)
                        }
                }
            } }
        }
    }
}

@Serializable private object TicketsRoute
@Serializable private object NewTicketRoute
@Serializable private object ChangePasswordRoute
@Serializable private data class TicketRoute(val id: Long)

/** Экраны для вошедшего пользователя. Вход и выход переключает [SessionState]. */
@Composable
private fun MainNavigation(user: User, openTicket: MutableStateFlow<Long?>) {
    val nav = rememberNavController()
    val pending by openTicket.collectAsStateWithLifecycle()
    LaunchedEffect(pending) {
        pending?.let {
            nav.navigate(TicketRoute(it)) { popUpTo<TicketsRoute>() }
            openTicket.value = null
        }
    }
    NavHost(nav, startDestination = TicketsRoute) {
        composable<TicketsRoute> {
            TicketListScreen(
                user = user,
                onOpen = { nav.navigate(TicketRoute(it)) },
                onCreate = { nav.navigate(NewTicketRoute) },
                onChangePassword = { nav.navigate(ChangePasswordRoute) },
            )
        }
        composable<NewTicketRoute> {
            NewTicketScreen(
                user = user,
                onBack = { nav.popBackStack() },
                onCreated = { id ->
                    nav.navigate(TicketRoute(id)) { popUpTo<TicketsRoute>() }
                },
            )
        }
        composable<TicketRoute> { entry ->
            TicketDetailScreen(id = entry.toRoute<TicketRoute>().id, user = user, onBack = { nav.popBackStack() })
        }
        composable<ChangePasswordRoute> {
            ChangePasswordScreen(forced = false, onDone = { nav.popBackStack() }, onBack = { nav.popBackStack() })
        }
    }
}

/** На Android 13+ уведомления нужно разрешить явно; спрашиваем один раз после входа. */
@Composable
private fun NotificationPermission() {
    if (Build.VERSION.SDK_INT < 33) return
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    val context = LocalContext.current
    LaunchedEffect(Unit) {
        if (ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS)
            != PackageManager.PERMISSION_GRANTED
        ) launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
    }
}
