# language: de
Funktionalität: Ersteinrichtung und Navigation im Dashboard

  Szenario: Die Suche bleibt zentriert, wenn sich der Kopfbereich ändert
    Angenommen das Dashboard hat einen langen Titel und zeigt Docker-Status und Abmelden an
    Wenn sich die Anzahl der sichtbaren Docker-Statuszähler ändert
    Dann bleibt das Suchfeld im Kopfbereich zentriert
    Wenn ich das Dashboard auf einem schmalen Bildschirm ansehe
    Dann steht die Suche in einer eigenen Zeile, ohne andere Bedienelemente zu überdecken

  Szenario: Ein neues Dashboard mit einem ersten Link einrichten
    Angenommen das Dashboard hat eine unveränderte Standardkonfiguration
    Wenn ich das Dashboard benenne, die erste Seite "Overview" nenne und einen Link hinzufüge
    Und ich die Einrichtung abschließe
    Dann sehe ich "Overview" und meinen Link
    Und die Einrichtung erscheint beim erneuten Öffnen nicht mehr

  Szenario: Die Standardwerte übernehmen
    Angenommen das Dashboard hat eine unveränderte Standardkonfiguration
    Wenn ich die Einrichtung überspringe
    Dann kann ich die Standardseite "Home" bearbeiten
    Und die Einrichtung erscheint beim erneuten Öffnen nicht mehr

  Szenario: Eine Suche zurücksetzen
    Angenommen die aktuelle Seite hat mehrere Kacheln
    Wenn ich einen Suchbegriff eingebe, der nur zu einer Kachel passt
    Und ich auf "Suche leeren" klicke
    Dann ist das Suchfeld leer und alle Kacheln der Seite sind wieder sichtbar

  Szenario: Keine Kacheln in der Lücke über dem schwebenden Kopfbereich
    Angenommen das Dashboard hat genug Kacheln zum Scrollen
    Wenn ich scrolle, bis Kacheln hinter dem fixierten Kopfbereich verschwinden
    Dann behält der Kopfbereich seinen üblichen Abstand zum oberen Fensterrand
    Und in der Lücke darüber ist nur der Seitenhintergrund sichtbar

  Szenario: Einen schlafenden Container ohne Kachel finden
    Angenommen Docker meldet einen beendeten Sablier-Container ohne Dashboard-Kachel
    Wenn ich die Anzahl der schlafenden Container auswähle
    Dann wird der Container mit Namen aufgelistet

  Szenario: Normalen Containerbetrieb von Ausnahmen unterscheiden
    Angenommen das Dashboard hat Kacheln für gesunde, laufende, schlafende und fehlerhafte Container
    Dann zeigen normal laufende Container einen grünen Statuspunkt ohne sichtbare Statusbeschriftung
    Und der genaue Status ist beim Überfahren mit der Maus und für Screenreader verfügbar
    Und schlafende und fehlerhafte Container haben weiterhin eine sichtbare Statusbeschriftung

  Szenario: Die Containerübersicht im Kopfbereich ausblenden
    Angenommen Docker ist verbunden und meldet 56 von 57 laufenden Containern
    Wenn ich unter "Darstellung" die Option "Begrüßung, Uhrzeit und Container-Übersicht anzeigen" deaktiviere
    Dann zeigt der Kopfbereich weder Begrüßung noch Uhrzeit noch Containerzahlen
    Wenn Docker danach nicht mehr erreichbar ist
    Dann zeigt der Kopfbereich den Hinweis "Docker nicht erreichbar"

  Szenario: Das Dashboard hinter der Containerliste bleibt stehen
    Angenommen das Dashboard ist nach unten gescrollt und die Liste laufender Container ist höher als das Fenster
    Wenn ich die Liste laufender Container öffne
    Dann bewegt Scrollen über dem Hintergrund das Dashboard nicht
    Und ich kann in der Containerliste scrollen, ohne das Dashboard zu scrollen
    Und nach dem Schließen steht das Dashboard an derselben Position

  Szenario: Persönliches Farbschema wählen und Bearbeiten über das Zahnrad öffnen
    Angenommen ein eingerichtetes Dashboard mit dunklem Standardthema
    Wenn ich die Einstellungen öffne und "Hell" wähle
    Dann ist das Dashboard auch nach dem Neuladen im selben Browser hell
    Wenn ich "Dashboard-Standard" wähle
    Dann kehrt das Dashboard zu seinem dunklen Standardthema zurück
    Wenn ich in den Einstellungen "Dashboard bearbeiten" wähle
    Dann ist die Bearbeitungsleiste sichtbar

  Szenario: Das Dashboard hinter dem Darstellungsdialog bleibt stehen
    Angenommen das Dashboard ist nach unten gescrollt
    Wenn ich den Darstellungsdialog öffne und über dem Hintergrund scrolle
    Dann bleibt der Hintergrund an derselben Scrollposition
    Und der Dialog selbst lässt sich scrollen, wenn sein Inhalt höher als das Fenster ist

  Szenario: Die Konfiguration als JSON in voller Höhe bearbeiten
    Angenommen ich bin im Bearbeitungsmodus
    Wenn ich "JSON" öffne
    Dann füllt das Eingabefeld die verfügbare Höhe des Dialogs aus
    Und der Dialog ist auf dem Desktop nicht breiter als 900 Pixel
    Und das Eingabefeld hat keinen Griff zum Ändern der Größe
    Und die Schaltflächen darunter bleiben rechtsbündig an ihrer Position

  Szenario: Lange Formulare behalten ihre Schaltflächen sichtbar
    Angenommen ich öffne "Kachel hinzufügen"
    Dann sind Auswahlfelder genauso hoch wie Texteingabefelder
    Und nur die Bezeichnung von Pflichtfeldern ist mit einem Stern markiert
    Und "Abbrechen" und "Fertig" bleiben beim Scrollen am unteren Rand des Dialogs sichtbar

  Szenario: Eine Seite ohne Neuladen entfernen
    Angenommen das Dashboard hat die Seiten "Overview" und "Maintenance"
    Wenn ich die Seite "Maintenance" lösche und speichere
    Dann zeigt das Dashboard ohne Neuladen nur noch "Overview"

  Szenario: Die Bearbeitungsleiste auf einem schmalen Bildschirm verwenden
    Angenommen das Dashboard ist auf Deutsch und in einem schmalen Fenster geöffnet
    Wenn ich den Bearbeitungsmodus öffne und zur letzten Gruppe scrolle
    Dann zeigt die Bearbeitungsleiste ihre Hinweise und alle Schaltflächen ohne horizontales Abschneiden
    Und die Bedienelemente der letzten Gruppe lassen sich vollständig über die Leiste scrollen

  Szenario: Die Version eines Docker-Builds abfragen
    Angenommen das Dashboard-Image wurde ohne Versionsangabe aus dem Repository gebaut
    Wenn ich "/healthz" von diesem Image abrufe
    Dann entspricht die gemeldete Version der ersten Zeile von "semver.txt"
    Und die Einstellungen zeigen dieselbe Version

  Szenario: Einen Build an seinem Startprotokoll erkennen
    Angenommen das Dashboard-Image wurde ohne Versionsangabe aus dem Repository gebaut
    Wenn das Dashboard mit "LABDASH_LOG_LEVEL=error" startet
    Dann enthält sein Startprotokoll die Version aus den Einstellungen
    Und gewöhnliche Info-Meldungen werden nicht protokolliert

  Szenario: Die Version eines Docker-Builds überschreiben
    Angenommen das Dashboard-Image wurde mit der Versionsangabe "0.1.0-beta.1" gebaut
    Wenn ich "/healthz" von diesem Image abrufe
    Dann ist die gemeldete Version "0.1.0-beta.1"
