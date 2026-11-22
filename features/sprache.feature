# language: de
Funktionalität: Sprache aus den Browsereinstellungen übernehmen

  Szenario: Ein Sprachwechsel lässt das gespeicherte Layout unverändert
    Angenommen ein eingerichtetes Dashboard und eine neue englischsprachige Browsersitzung ohne gespeicherte Sprachwahl
    Wenn ich das Dashboard öffne
    Dann ist die Oberfläche auf Englisch
    Wenn ich in den Einstellungen Deutsch wähle und eine Gruppe "Friday services" hinzufüge
    Und ich das Layout speichere und das Dashboard neu lade
    Dann ist die Oberfläche auf Deutsch
    Wenn ich in den Einstellungen Englisch wähle
    Dann heißt die Gruppe weiterhin "Friday services"

  Szenario: Deutsch in einem frischen deutschsprachigen Browser verwenden
    Angenommen ein Browser, der Deutsch bevorzugt und keine gespeicherte Dashboard-Sprache hat
    Wenn ich das Dashboard öffne
    Dann ist die Oberfläche auf Deutsch
    Wenn ich in den Einstellungen Englisch wähle und das Dashboard neu lade
    Dann ist die Oberfläche auf Englisch
    Wenn ich in den Einstellungen "Browsersprache" wähle
    Dann ist die Oberfläche auf Deutsch
