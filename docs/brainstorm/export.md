Oui. Pour exporter des rapports NetWorker en **CLI**, l’option la plus pratique pour les rapports NMC est généralement **`gstclreport`**, qui permet d’exécuter un rapport, de choisir le type de vue, et d’exporter en CSV, HTML, PDF ou autre format. [youtube](https://www.youtube.com/watch?v=YTr7HyTge_Y)

## Commande de base

La structure ressemble à ceci :

```bash
gstclreport -u <user> -P <password> \
-r "<chemin_du_rapport>" \
-v <table|chart> -x <csv|html|pdf|print> \
-f <nom_fichier>
```

Le chemin complet du binaire est souvent `/opt/lgotnmc/bin/gstclreport`, et il faut parfois définir `JAVA_HOME` avant de l’utiliser. [youtube](https://www.youtube.com/watch?v=YTr7HyTge_Y)

## Exemples utiles

Pour un export en CSV d’un rapport de type **Client Summary** :

```bash
/opt/lgotnmc/bin/gstclreport \
-u admin -P 'password' \
-r "/Reports/NetWorker Backup Statistics/Client Summary" \
-v table -x csv -f client_summary.csv
```

Pour un rapport **Group Summary** en HTML :

```bash
/opt/lgotnmc/bin/gstclreport \
-u admin -P 'password' \
-r "/Reports/NetWorker Backup Statistics/Group Summary/Daily Group Report" \
-v chart -c pie -x html -f daily_groups.html \
-C "Save Time" "1 day"
```

Ces exemples correspondent au mode d’usage documenté pour `gstclreport`, avec paramètres de filtre via `-C` comme **Save Time**, **Group Name** ou **Server Name**. [youtube](https://www.youtube.com/watch?v=YTr7HyTge_Y)

## Rapports à exporter en priorité

Pour ton sizing, je te conseille d’exporter d’abord :

- **Client Summary** pour la capacité par client. [youtube](https://www.youtube.com/watch?v=YTr7HyTge_Y)
- **Group Summary** pour les fenêtres et volumes par groupe. [youtube](https://www.youtube.com/watch?v=YTr7HyTge_Y)
- **Recovery / Restore reports** si tu veux valider les besoins de restauration. [nsrd](https://nsrd.info/blog/2009/12/02/recovery-reporting-comes-to-networker/)
- **Capacity measurement** si ton environnement NetWorker le supporte. [dell](https://www.dell.com/support/kbdoc/en-us/000029555/esg118514-networker-source-capacity-licensing-how-to-calculate-source-capacity)

## Alternative en ligne de commande

Pour des extractions de données brutes, **`mminfo`** est très utile, par exemple pour lister les save sets et volumes protégés ; Dell et la documentation NetWorker indiquent que `mminfo` sert au reporting des media et save sets. [techpubs.jurassic](https://techpubs.jurassic.nl/library/manuals/1000/007-1458-060/sgi_html/apd.html)

Exemple typique :

```bash
mminfo -avot -q client=<CLIENT> -r client,name,ssid,sssize,level,volume
```

Cela ne produit pas un “rapport NMC” formatté, mais c’est très efficace pour construire ton propre export CSV et faire du sizing dans Excel/Power BI. [dell](https://www.dell.com/community/en/conversations/networker/view-total-backup-size-protected-by-networker/647f5fc0f4ccf8a8deb13723)

## Recommandation pratique

Pour travailler vite, je ferais :

1. un export **gstclreport** pour les rapports standards,
2. un export **mminfo** pour les données détaillées,
3. une consolidation en CSV pour calculer capacité, croissance et rétention. [techpubs.jurassic](https://techpubs.jurassic.nl/library/manuals/1000/007-1458-060/sgi_html/apd.html)

Je peux te préparer juste après un **script bash prêt à l’emploi** avec 3 exports NetWorker: client, group et capacity, au format CSV.
