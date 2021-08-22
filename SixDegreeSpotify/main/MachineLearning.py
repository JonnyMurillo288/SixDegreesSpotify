import base64
import platform
from urllib.parse import urlencode
import datetime
from datetime import timezone
import time
import random
import geckodriver_autoinstaller
import requests
import os, sys
import logging
import pandas as pd
import json
import mysql.connector as sql
import random
from statistics import quantiles
import geckodriver_autoinstaller 
import sys
sys.path.append("..")
geckodriver_autoinstaller.install()
from SpotifyAuth import SpotifyAPI, UserData, getConfig, tokenFromRefresh
from sqlalchemy import create_engine

client_id = '62d46cea622741b1b6013c64688e2dfa'
client_secret = '1a863a23884b410c8b55d2b324eb7c84'
redirect_uri = 'http://song-in-playlist-finder/callback'


class Recommendation(SpotifyAPI):
    """Set nonetype variables that will change for each user."""
    recomended_tracks = []  # Recomended tracks have ID and URI for adding to playlist


    def __init__(self,token,db, *args, **kwargs):
        super().__init__(token,db,*args,**kwargs)
        self.access_token = token
        self.s_user_id = self.getUserID()
        self.db = db
        self.c = self.db.cursor()


    def decision_tree_recommendation(self,train_df,df_for_tree=None,samples_split=25):
        """Run decision tree with recommended songs before adding them
        :returns list for adding to playlist
        """

        from sklearn import tree
        from sklearn.metrics import accuracy_score
        from sklearn.model_selection import train_test_split
        from sklearn.preprocessing import OrdinalEncoder


        tempo = train_df['tempo']
        try:
            train_df['tempo_0_1'] = (tempo - tempo.min()) / (tempo.max() - tempo.min())

            tempo = df_for_tree['tempo']
            df_for_tree['tempo_0_1'] = (tempo - tempo.min()) / (tempo.max() - tempo.min())

        except TypeError:
            # if the tree did not get anything passed to it
            self.cluster_playlists(random.randint(6,10))
            return 
        
        clf = tree.DecisionTreeClassifier(min_samples_split=samples_split)
        try:
            X = train_df[['danceability', 'energy', 'key',
                                'speechiness', 'acousticness', 'instrumentalness', 'liveness',
                                'valence', 'tempo_0_1', 'popularity','clustered']].fillna(0.5)
            y = train_df[['playlist_track']]

            P = df_for_tree[['danceability', 'energy', 'key',
                                'speechiness', 'acousticness', 'instrumentalness', 'liveness',
                                'valence', 'tempo_0_1', 'popularity','clustered']].fillna(0.5)
            q = df_for_tree[['playlist_track']]
        except TypeError:
            # if the tree did not get anything passed to it
            self.cluster_playlists(random.randint(6,10))
            return 


        X_train, _, y_train, _ = train_test_split(X, y, random_state=84, test_size=.25)

        clf.fit(X, y)
        y_pred_test = clf.predict(X_train)

        score = accuracy_score(y_pred_test,y_train)
        logging.info(f'SCORE + {score}')
        if score < .85:
            self.decision_tree_recommendation(df_for_tree, samples_split=random.randint(10,100))

        pred = clf.predict(P)

        df_for_tree['pred'] = pred

        pred_like = df_for_tree.loc[df_for_tree['pred'] == 1]

        print(f'adding transfer to playlist\n{pred_like["track_id"]}')
        n,url = [], []
        dat = self.getDetails(pred_like['track_id'])
        if type(dat) == int:
            print("Error {} with getting track details".format(dat))
            self.cluster_playlists(random.randint(6,10))
            return 

        for data in dat:
            n.append(data['name'])
            url.append(data['image_url'])
        pred_like['track_name'], pred_like['image_url'] = n, url
        pred_like['id'] = self.s_user_id

        engine = create_engine("mysql://jonny:Yankees162162@localhost:3306/Spotify")
        pred_like.to_sql(name='sufficientAddToPlaylist',con=engine,if_exists='replace')
        self.db.commit()

        # send the tracks 
        f = open("recommendedTracks.txt","w+")
        for _,row in pred_like[['track_name','image_url','track_id']].iterrows():
            f.write("PlaceHolderPlaylist," + row['track_name']+ "," +row['image_url'] + "," + row["track_id"] + "\n")
        f.close()
        return pred_like

    def getDetails(self,tracks):
        """ Get the detail for the track that are not gotten from tracks features """
        # name, image_url
        track_data = []
        ud = UserData(self.access_token,self.db,setup=False)
        for track in tracks:
            data = ud.trackDetails(track)
            if type(data) == int:
                print("Error getting data",data)
                self.getDetails(tracks)
                break
            track_data.append({"name": data['name'], "image_url":data['album']['images'][1]['url']})
        return track_data

    def cluster_playlists(self,n_clusters=8):
        """User a clustering analysis on playlists so we can add a second layer of decision tree recomendation
        Use to make sure we have similar songs to certain playlists that the users listen to the most.
        """
        from sklearn.cluster import AgglomerativeClustering
        from sklearn.preprocessing import OrdinalEncoder

        print("Clustering songs into {} clusters".format(n_clusters))

        x = pd.read_sql(sql="SELECT playlist_name, playlist_tracks FROM userPlaylists",con=self.db) # get all tracks from userPlaylists DB
        rec_df = pd.read_sql(sql="SELECT * FROM recommendedTracksFeatures",con=self.db)

        d = {} # empty dict to fill for new df
        for i,_ in x.iterrows(): #Unpack and load tracks from list
            new = x.iloc[i][1]
            tracks = json.loads(new)
            for track in tracks:
                d[track] = x.iloc[i][0]
                
        new_df = pd.DataFrame.from_dict(d,orient='index',columns=['playlist']) #df with index of track and playlist in col

        enc = OrdinalEncoder() #Encode playlists for clustering
        enc.fit(new_df[['playlist']])
        new_df['playlist_code'] = enc.transform(new_df[['playlist']])

        feat = pd.read_sql(sql="SELECT * FROM userTrackFeatures",con=self.db)
        feat = feat.join(new_df,on='track_id')
        feat['recommended'] = 0
        rec_df['recommended'] = int(1)

        enc = OrdinalEncoder()
        
        pre_cluster = pd.concat([feat,rec_df],axis=0)

        tempo = pre_cluster['tempo']
        try:
            tempo = pre_cluster['tempo']
            pre_cluster['tempo_0_1'] = (tempo - tempo.min()) / (tempo.max() - tempo.min())

        except TypeError:
            pass

        pre_cluster['genre_code'] = enc.fit_transform(pre_cluster[['genre']])

        clust = AgglomerativeClustering(n_clusters=n_clusters)

        X = pre_cluster[['danceability', 'energy', 'key',
                'speechiness', 'acousticness', 'instrumentalness', 'liveness',
                'valence', 'tempo_0_1', 'popularity', 'top_track','playlist_code','genre_code']].fillna(0.5)

        pred = clust.fit_predict(X)

        pre_cluster['clustered'] = pred

        clus_ls = pre_cluster['clustered'].value_counts()[:2].index.tolist() #send clusters to list ordered by most popular clusters

        con_ls = [] # list to add df to before concat

        clus_ls.append(random.randint(2,n_clusters))
        for i in clus_ls:
            con_ls.append(pre_cluster.loc[pre_cluster['clustered']==i])
        
        pre_tree = pd.concat(con_ls,axis=0)

        df_to_tree = pre_tree.loc[pre_tree['recommended'] == 0]
        recommended_tracks = pre_tree.loc[pre_tree['recommended'] == 1]

        done = False

        self.decision_tree_recommendation(train_df=df_to_tree,df_for_tree=recommended_tracks,samples_split=25)
        
        done = True
        return done
        


if __name__ == '__main__':
    cnx = sql.connect(user='jonny',database='Spotify',password='Yankees162162')
    c = cnx.cursor()

    conf = getConfig()
    if datetime.datetime.now().isoformat() > conf['expiry']:
        tokenFromRefresh()
        conf = getConfig()
    a = Recommendation(conf['access_token'],cnx,setup=False)
    a.cluster_playlists(5)
