Oui — pour un sizing de **plateforme de sauvegarde**, les rapports les plus utiles sont ceux qui te donnent la **capacité source réelle**, la **consommation de stockage**, la **réussite des jobs**, et la **récupérabilité**. Avec NetWorker, l’idée est de partir de rapports orientés charge, croissance et restauration plutôt que de seulement compter les jobs. [learning.dell](https://learning.dell.com/content/dam/dell-emc/documents/en-us/2014KS_Panchanathan-How_Analytics_Can_Help_Backup_Administrators.pdf)

## Rapports à prioriser

- **Client summary / par client** : volume protégé par client, répartition des workloads, croissance mensuelle, taux de succès. C’est la base pour estimer la capacité à protéger par application ou par serveur. [support.liveoptics](https://support.liveoptics.com/hc/en-us/articles/7444263744795-Networker-API-Excel-Definitions)
- **Group summary / par policy ou groupe** : utile pour voir quels groupes consomment le plus de fenêtre de backup, quels jobs durent longtemps, et où se concentrent les échecs. [veritas](https://www.veritas.com/support/en_US/doc/140388457-166019496-0/pgfId-131140-166019496)
- **Capacity measurement / source capacity** : c’est le rapport le plus important pour un sizing, car NetWorker peut estimer la capacité totale protégée sur une fenêtre de 60 jours en prenant le plus grand full par client et par type de données. [dell](https://www.dell.com/support/kbdoc/en-us/000029555/esg118514-networker-source-capacity-licensing-how-to-calculate-source-capacity)
- **Backup failures and job statistics** : taux d’échec, jobs longs, stalled jobs, throughput moyen. Cela aide à prévoir la marge réseau, disque et fenêtre d’exécution. [learning.dell](https://learning.dell.com/content/dam/dell-emc/documents/en-us/2014KS_Panchanathan-How_Analytics_Can_Help_Backup_Administrators.pdf)
- **Recover details / restore reports** : indispensable pour valider les besoins de restauration, les volumes réellement restaurés et les temps de recovery. [nsrd](https://nsrd.info/blog/2017/12/19/basics-prior-recovery-details/)

## Ce que je te conseille de produire

1. **Vue capacité**
   - capacité protégée totale par client.
   - capacité par type de workload.
   - croissance sur 30 / 60 / 90 jours.
   - top 10 des plus gros producteurs de données. [support.liveoptics](https://support.liveoptics.com/hc/en-us/articles/7444263744795-Networker-API-Excel-Definitions)

2. **Vue opérationnelle**
   - nombre de jobs réussis / échoués.
   - durée moyenne et maximale des backups.
   - jobs stables / lents / bloqués.
   - fenêtre de sauvegarde utilisée vs disponible. [veritas](https://www.veritas.com/support/en_US/doc/140388457-166019496-0/pgfId-131140-166019496)

3. **Vue restauration**
   - nombre de restores.
   - volume restauré.
   - délais moyens de restauration.
   - succès / échec des restores. [nsrd](https://nsrd.info/blog/2009/12/02/recovery-reporting-comes-to-networker/)

4. **Vue architecture**
   - stockage primaire vs rétention.
   - déduplication / compression si disponible.
   - croissance projetée à 12 / 24 mois.
   - marge de sécurité recommandée. [dell](https://www.dell.com/support/manuals/en-us/networker/networker_administration_guide_19.12/front-end-capacity-estimation?guid=guid-01bc3a26-1872-4490-9464-383787c22dff&lang=en-us)

## Format de rapport utile

Je te recommande un pack de 3 livrables :

- **Executive summary** d’une page avec chiffres clés.
- **Annexe technique** avec tables par client, groupe, stockage, et erreurs.
- **Projection capacity** avec hypothèses de croissance et besoin cible. [backup-eagle](https://www.backup-eagle.com/backup-software/dell-emc-networker/)

## Si tu veux quelque chose de concret

Tu peux bâtir ton rapport avec ces indicateurs :

| Indicateur | Pourquoi il est utile |
|---|---|
| Source capacity 60 jours | Base de sizing principale  [dell](https://www.dell.com/support/kbdoc/en-us/000029555/esg118514-networker-source-capacity-licensing-how-to-calculate-source-capacity). |
| Volume protégé par client | Identifie les plus gros consommateurs  [support.liveoptics](https://support.liveoptics.com/hc/en-us/articles/7444263744795-Networker-API-Excel-Definitions). |
| Taux de succès des jobs | Mesure la santé de la plateforme  [learning.dell](https://learning.dell.com/content/dam/dell-emc/documents/en-us/2014KS_Panchanathan-How_Analytics_Can_Help_Backup_Administrators.pdf). |
| Durée moyenne / max des backups | Vérifie la faisabilité dans la fenêtre  [veritas](https://www.veritas.com/support/en_US/doc/140388457-166019496-0/pgfId-131140-166019496). |
| Statistiques de restauration | Valide le besoin métier réel  [nsrd](https://nsrd.info/blog/2009/12/02/recovery-reporting-comes-to-networker/). |

## Approche pratique

Si tu as accès à NetWorker, commence par extraire :

- les **reports client summary**,
- le **capacity measurement**,
- les **backup failures / running jobs**,
- les **recover details**. [nsrd](https://nsrd.info/blog/2009/12/02/recovery-reporting-comes-to-networker/)

Ensuite, consolide dans Excel ou Power BI avec des courbes de croissance et un calcul de capacité cible à 12 ou 24 mois. Pour le sizing, je partirais avec une marge de sécurité sur la capacité projetée et j’alignerais ensuite la fenêtre backup/restore sur les SLA du client. [reseau.uquebec](https://reseau.uquebec.ca/system/files/documents/guide-plan-sauvegarde-v1.2-20240419.pdf)

Je peux aussi te proposer un **modèle de rapport de sizing NetWorker** prêt à remplir, avec les onglets et les KPI exacts à extraire.
