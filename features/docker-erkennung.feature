# language: de
Funktionalität: Container-Kacheln aus Docker-Labels erstellen und bearbeiten

  Szenario: Homepage-Labels nach dem Update ausdrücklich aktivieren
    Angenommen eine bestehende Konfiguration enthält "homepage" nur in "labelPrefixes"
    Und ein Container hat nur Homepage-Labels
    Und ein anderer Container hat passende labdash- und Homepage-Labels
    Dann fehlt die automatisch erzeugte Kachel für den ersten Container
    Und der zweite Container erscheint mit seinen labdash-Angaben
    Wenn ich im Bearbeitungsmodus unter Docker-Erkennung die Homepage-Option aktiviere und speichere
    Dann erscheint auch die Kachel für den ersten Container
    Und in der Konfiguration steht "discovery.homepage" auf "true"

  Szenario: Eine übernommene Homepage-Kachel behalten
    Angenommen die Homepage-Erkennung ist aktiviert
    Wenn ich eine automatisch erkannte Homepage-Kachel übernehme und umbenenne
    Und ich die Homepage-Erkennung deaktiviere und speichere
    Dann bleibt die umbenannte Kachel mit ihrer Container-Verknüpfung erhalten
    Und andere automatisch erzeugte Homepage-Kacheln verschwinden

  Szenario: Eine Kachel für einen Container ohne Labels anlegen
    Angenommen ein laufender Container ist nicht mit Dashboard-Labels versehen
    Wenn ich in einer Gruppe eine Kachel hinzufüge und den Container verknüpfe
    Und ich das Dashboard speichere
    Dann zeigt die gespeicherte Kachel den Live-Status des Containers

  Szenario: Eine Kachel manuell in einer erkannten Gruppe hinzufügen
    Angenommen alle Gruppen auf der Seite stammen aus Docker-Labels
    Und ich bin im Bearbeitungsmodus
    Wenn ich in der Gruppe "Media" auf "Kachel zu „Media“ hinzufügen" klicke
    Und ich einen Link "NAS" mit der Adresse "https://nas.example.org" speichere
    Dann erscheint "NAS" in der Gruppe "Media"
    Und die erkannten Kacheln der Gruppe bleiben sichtbar

  Szenario: Erkannte Kacheln wirken außerhalb des Bearbeitungsmodus wie normale Kacheln
    Angenommen eine Kachel stammt aus Docker-Labels
    Wenn ich das Dashboard ansehe
    Dann hat die Kachel denselben durchgehenden Rahmen wie meine eigenen Kacheln
    Wenn ich den Bearbeitungsmodus öffne
    Dann behält die Kachel ihren durchgehenden Rahmen
    Und sie ist als „Aus Docker-Labels“ gekennzeichnet
    Und nur der Platzhalter „Kachel hinzufügen“ ist gestrichelt umrandet

  Szenario: Eine Kachel über die Bearbeitungsleiste hinzufügen
    Angenommen alle Kacheln auf der Seite stammen aus Docker-Labels
    Wenn ich den Bearbeitungsmodus öffne
    Und ich in der Bearbeitungsleiste auf "Kachel" klicke
    Dann öffnet sich der Dialog "Kachel hinzufügen" für die erste Gruppe der Seite

  Szenario: Ohne Gruppe zuerst eine Gruppe anlegen
    Angenommen die Seite hat keine Gruppen
    Wenn ich im Bearbeitungsmodus in der Bearbeitungsleiste auf "Kachel" klicke
    Dann sehe ich den Hinweis "Lege zuerst eine Gruppe an."
